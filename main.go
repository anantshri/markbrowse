package main

import (
	"flag"
	"fmt"
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
	log.Printf("mdview serving %s on http://localhost%s", rootDir, addr)
	log.Fatal(http.ListenAndServe(addr, h))
}

func loadCustomCSS(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading custom CSS: %w", err)
	}
	return string(data), nil
}
