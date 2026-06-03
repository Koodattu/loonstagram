package web

import (
	"embed"
	"io/fs"
)

//go:embed templates/*.html static/*
var FS embed.FS

func StaticFS() fs.FS {
	static, err := fs.Sub(FS, "static")
	if err != nil {
		panic(err)
	}
	return static
}
