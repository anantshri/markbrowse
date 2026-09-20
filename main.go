package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"time"
)

var version = "dev"

func getVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" {
			return v
		}
	}
	return "dev"
}

// defaultListenHost is loopback: the server is unauthenticated, so it must
// not be reachable from the network unless the operator explicitly passes
// --listen 0.0.0.0 (or another address).
const defaultListenHost = "127.0.0.1"

func main() {
	port := flag.Int("port", 8080, "port to listen on")
	listenHost := flag.String("listen", defaultListenHost, "IP or hostname to bind (e.g. 0.0.0.0 to expose on the network)")
	cssPath := flag.String("css", "", "path to custom CSS file (replaces built-in stylesheet)")
	rawHTML := flag.Bool("raw-html", false, "render raw HTML in markdown unescaped and allow javascript:/data: URLs (only for content you trust)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("markbrowse %s\n", getVersion())
		os.Exit(0)
	}

	var rootDir string
	if args := flag.Args(); len(args) > 0 {
		abs, err := filepath.Abs(args[0])
		if err != nil {
			log.Fatalf("invalid directory: %v", err)
		}
		rootDir = abs
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			log.Fatal(err)
		}
		rootDir = cwd
	}

	info, err := os.Stat(rootDir)
	if err != nil {
		log.Fatalf("cannot access directory: %v", err)
	}
	if !info.IsDir() {
		log.Fatalf("%s is not a directory", rootDir)
	}
	if err := validateReadable(rootDir); err != nil {
		log.Fatalf("cannot serve %s: %v", rootDir, err)
	}

	customCSS, err := loadCustomCSS(*cssPath)
	if err != nil {
		log.Fatal(err)
	}

	h := &fileHandler{
		root:      rootDir,
		md:        newMarkdownConverter(rootDir, *rawHTML),
		customCSS: customCSS,
	}

	listener, actualPort, err := listenWithFallback(*listenHost, *port)
	if err != nil {
		log.Fatal(err)
	}

	// JoinHostPort brackets IPv6 literals correctly; Sprintf("%s:%d") does not.
	addr := net.JoinHostPort(*listenHost, strconv.Itoa(actualPort))
	srv := &http.Server{
		Addr:         addr,
		Handler:      h,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	log.Printf("markbrowse serving %s on http://%s", rootDir, net.JoinHostPort(displayHost(*listenHost), strconv.Itoa(actualPort)))
	log.Fatal(srv.Serve(listener))
}

// displayHost turns wildcard bind addresses into something a browser can
// actually open ("0.0.0.0" and "::" are not browsable URLs).
func displayHost(host string) string {
	if host == "" || host == "0.0.0.0" || host == "::" {
		return "localhost"
	}
	return host
}

func listenWithFallback(host string, port int) (net.Listener, int, error) {
	// nosemgrep: go.lang.security.audit.net.bind_all.avoid-bind-to-all-interfaces -- binds the operator-requested --listen address; loopback by default
	l, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err == nil {
		return l, port, nil
	}
	log.Printf("error: port %d unavailable: %v", port, err)

	start := port + 1
	if start < 10000 {
		start = 10000
	}
	for p := start; p <= 65535; p++ {
		// nosemgrep: go.lang.security.audit.net.bind_all.avoid-bind-to-all-interfaces -- fallback scan binds the same operator-requested host; loopback by default
		l, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(p)))
		if err == nil {
			log.Printf("falling back to port %d", p)
			return l, p, nil
		}
	}
	return nil, 0, fmt.Errorf("no available 5-digit port found above %d", port)
}

func loadCustomCSS(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path comes from user's -css flag
	if err != nil {
		return "", fmt.Errorf("reading custom CSS: %w", err)
	}
	return string(data), nil
}

// validateReadable confirms the root directory can actually be read before
// the server starts, so permission problems fail fast with a clear message
// instead of surfacing as per-request errors after startup.
func validateReadable(rootDir string) error {
	if _, err := os.ReadDir(rootDir); err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied reading directory (check read+execute permissions): %w", err)
		}
		return fmt.Errorf("reading directory: %w", err)
	}
	return nil
}
