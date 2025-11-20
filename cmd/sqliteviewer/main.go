package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"sqliteviewer/internal/server"
)

func main() {
	dbPath := flag.String("db", "", "Path to the SQLite file to inspect")
	addr := flag.String("addr", ":8080", "Address for the HTTP server")
	staticDir := flag.String("static", "", "Optional directory with custom frontend assets (defaults to embedded build)")
	flag.Parse()

	if *dbPath == "" {
		log.Fatal("missing required -db flag pointing to a SQLite file")
	}

	if err := ensureFileExists(*dbPath); err != nil {
		log.Fatalf("cannot access database file: %v", err)
	}

	var staticFS http.FileSystem
	var err error
	if *staticDir != "" {
		if err := ensureDirExists(*staticDir); err != nil {
			log.Fatalf("invalid static directory: %v", err)
		}
		staticFS = http.Dir(*staticDir)
	} else {
		staticFS, err = server.EmbeddedStatic()
		if err != nil {
			log.Fatalf("failed to load embedded assets: %v", err)
		}
	}

	srv, err := server.New(*dbPath, staticFS)
	if err != nil {
		log.Fatalf("failed to initialize server: %v", err)
	}

	log.Printf("Starting sqliteviewer on %s (db: %s)", *addr, *dbPath)
	if err := srv.Run(*addr); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

func ensureFileExists(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return &os.PathError{
			Op:   "stat",
			Path: path,
			Err:  os.ErrInvalid,
		}
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		log.Printf("Using database file: %s", abs)
	}
	return nil
}

func ensureDirExists(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return &os.PathError{
			Op:   "stat",
			Path: path,
			Err:  os.ErrInvalid,
		}
	}
	return nil
}
