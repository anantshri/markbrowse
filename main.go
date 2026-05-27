package main

import (
	"flag"
	"fmt"
	"time"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	port := flag.Int("port", 8080, "port to listen on")
	cssPath := flag.String("css", "", "path to custom CSS file (replaces built-in stylesheet)")
	flag.Parse()

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

	customCSS, err := loadCustomCSS(*cssPath)
	if err != nil {
		log.Fatal(err)
	}

	h := &fileHandler{
		root:      rootDir,
		md:        newMarkdownConverter(rootDir),
		customCSS: customCSS,
	}

	addr := fmt.Sprintf(":%d", *port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      h,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	log.Printf("mdview serving %s on http://localhost%s", rootDir, addr)
	log.Fatal(srv.ListenAndServe())
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
