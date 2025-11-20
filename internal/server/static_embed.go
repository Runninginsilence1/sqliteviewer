package server

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed ui/dist/*
var embeddedStatic embed.FS

// EmbeddedStatic returns an http.FileSystem backed by the embedded frontend build.
func EmbeddedStatic() (http.FileSystem, error) {
	sub, err := fs.Sub(embeddedStatic, "ui/dist")
	if err != nil {
		return nil, err
	}
	return http.FS(sub), nil
}
