package packer

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/blackss2/utility/convert" //TEMP

	"github.com/blackss2/devfarm/common"
	"github.com/blackss2/devfarm/utils"
)

var (
	ErrNotExistSource    = errors.New("not exist source")
	ErrNotSupportCommand = errors.New("not support command")
	ErrUnknownPackage    = errors.New("unknown packages")
)

func PackSourceZip(Command string, BuildFlags []string, Packages string) ([]byte, error) {
	if Command != "install" && Command != "build" {
		return nil, ErrNotSupportCommand
	}

	curDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	zw := zip.NewWriter(&buffer)
	ignorePrefix := []string{
		"bin/",
		"pkg/",
		"src/",
		"__resources/",
		".git/",
		".settings/",
		".project",
	}
	err = utils.AddDirToZip(zw, curDir, "__resources", ignorePrefix)
	if err != nil {
		return nil, err
	}

	srcPath, err := filepath.Abs(filepath.Clean(fmt.Sprintf("%s/%s", curDir, Packages)))
	if err != nil {
		return nil, err
	}

	println(curDir, srcPath)

	_, err = os.Stat(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotExistSource
		}
	}

	prefix := filepath.Clean(Packages)
	if prefix == "..." || prefix == "." {
		prefix = ""
	}
	err = utils.AddDirToZip(zw, srcPath, "src/"+prefix, nil)
	if err != nil {
		return nil, err
	}

	importPaths, err := utils.GetTotalImportList(srcPath, os.Getenv("GOPATH"), curDir)
	if err != nil {
		return nil, err
	}

	goPaths := []string{os.Getenv("GOPATH"), curDir}
	for _, v := range importPaths {
		for _, goPath := range goPaths {
			if strings.HasPrefix(v, goPath) {
				rel, err := filepath.Rel(goPath+"/src", v)
				if err != nil {
					return nil, err
				}
				err = utils.AddDirToZip(zw, v, "src/"+rel, nil)
				if err != nil {
					return nil, err
				}
				break
			}
		}
	}

	fw, err := zw.Create("manifest.json")
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(&common.Manifest{
		Command:    Command,
		BuildFlags: BuildFlags,
		Packages:   Packages,
	})
	if err != nil {
		return nil, err
	}
	fw.Write(data)
	zw.Close()

	return buffer.Bytes(), nil
}
