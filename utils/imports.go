package utils

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
)

func GetTotalImportList(path string, goPaths ...string) ([]string, error) {
	if len(goPaths) == 0 {
		goPaths = []string{os.Getenv("GOPATH")}
	}

	importList, err := GetImportList(path, nil, goPaths)
	if err != nil {
		return nil, err
	}
	pathHash := make(map[string]bool)
	for len(importList) > 0 {
		subList := make([]string, 0)
		for _, v := range importList {
			list, err := GetImportList(v, pathHash, goPaths)
			if err != nil {
				return nil, err
			}
			if len(list) > 0 {
				subList = append(subList, list...)
			}
		}
		importList = subList
	}
	totalImportList := make([]string, 0)
	for key, _ := range pathHash {
		totalImportList = append(totalImportList, key)
	}
	return totalImportList, nil
}

func GetImportList(path string, pathHash map[string]bool, goPaths []string) ([]string, error) {
	var err error
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	importList := make([]string, 0)
	isVisited := false
	if pathHash != nil {
		if _, has := pathHash[path]; has {
			isVisited = true
		}
	}

	if !isVisited {
		if _, err := os.Stat(path); err == nil {
			filepath.Walk(path, func(subpath string, finfo os.FileInfo, err error) error {
				if !finfo.IsDir() {
					ext := filepath.Ext(subpath)
					if ext == ".go" {
						fset := token.NewFileSet()
						f, err := parser.ParseFile(fset, subpath, nil, 0)
						if err != nil {
							return err
						}

						for _, v := range f.Imports {
							for _, goPath := range goPaths {
								Len := len(v.Path.Value)
								src := fmt.Sprintf("%s/src/%s", goPath, v.Path.Value[1:Len-1])
								_, err := os.Stat(src)
								if err != nil {
									if os.IsNotExist(err) {
										continue
									} else {
										return err
									}
								}
								importList = append(importList, src)
							}
						}
					}
				}
				return nil
			})

			if pathHash != nil {
				pathHash[path] = true
			}
		}
	}
	return importList, nil
}
