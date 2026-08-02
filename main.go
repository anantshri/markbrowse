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

func main() {
	port := flag.Int("port", 8080, "port to listen on")
	cssPath := flag.String("css", "", "path to custom CSS file (replaces built-in stylesheet)")
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
		md:        newMarkdownConverter(rootDir),
		customCSS: customCSS,
	}

	listener, actualPort, err := listenWithFallback(*port)
	if err != nil {
		log.Fatal(err)
	}

	addr := fmt.Sprintf(":%d", actualPort)
	srv := &http.Server{
		Addr:         addr,
		Handler:      h,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	log.Printf("markbrowse serving %s on http://localhost%s", rootDir, addr)
	log.Fatal(srv.Serve(listener))
}

func listenWithFallback(port int) (net.Listener, int, error) {
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err == nil {
		return l, port, nil
	}
	log.Printf("error: port %d unavailable: %v", port, err)

	start := port + 1
	if start < 10000 {
		start = 10000
	}
	for p := start; p <= 65535; p++ {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
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

// validateReadable confirms the root directory can actually be read before the
// server starts, so permission problems fail fast with a clear message instead
// of surfacing as per-request errors. It checks the directory itself and, when
// present, a README/index file so the most common serve path is verified.
func validateReadable(rootDir string) error {
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied reading directory (check read+execute permissions): %w", err)
		}
		return fmt.Errorf("reading directory: %w", err)
	}
	for _, name := range []string{"README.md", "readme.md", "INDEX.md", "index.md"} {
		indexPath := filepath.Join(rootDir, name)
		if info, err := os.Stat(indexPath); err == nil && !info.IsDir() {
			data, err := os.ReadFile(indexPath) // #nosec G304 -- fixed candidate index name
			if err != nil {
				if os.IsPermission(err) {
					return fmt.Errorf("permission denied reading %s: %w", name, err)
				}
				return fmt.Errorf("reading %s: %w", name, err)
			}
			_ = data
			return nil
		}
	}
	// No index file: verify at least the first entry can be inspected.
	if len(entries) > 0 {
		if _, err := entries[0].Info(); err != nil {
			if os.IsPermission(err) {
				return fmt.Errorf("permission denied accessing %q: %w", entries[0].Name(), err)
			}
			return fmt.Errorf("accessing %q: %w", entries[0].Name(), err)
		}
	}
	return nil
}
