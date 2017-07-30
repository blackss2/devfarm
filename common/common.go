package common

import (
	"io"
)

type Manifest struct {
	Command    string   `json:"command"`
	BuildFlags []string `json:"build_flags"`
	Packages   string   `json:"packages"`
}

type SourceFile struct {
	Path       string
	ReadCloser io.ReadCloser
}

type BinaryFile struct {
	Path       string
	ReadCloser io.ReadCloser
}
