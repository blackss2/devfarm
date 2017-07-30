package utils

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func AddDirToZip(zw *zip.Writer, localPath string, prefix string, ignorePrefix []string) error {
	if len(prefix) > 0 {
		prefix = prefix + "/"
	}
	prefix = filepath.ToSlash(prefix)
	err := filepath.Walk(localPath, func(path string, info os.FileInfo, err error) error {
		rel, err := filepath.Rel(localPath, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		path = filepath.ToSlash(path)
		if info != nil && !info.IsDir() {
			for _, v := range ignorePrefix {
				if strings.HasPrefix(filepath.ToSlash(rel), filepath.ToSlash(v)) {
					return nil
				}
			}
			fw, err := zw.Create(prefix + rel)
			if err != nil {
				return err
			}
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(fw, file)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}
