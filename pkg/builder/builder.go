package builder

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/blackss2/devfarm/common"
	"github.com/blackss2/devfarm/utils"
)

var (
	ErrNotSupportCommand = errors.New("not support command")
)

func BuildFromSourceZip(data []byte) ([]byte, error) {
	manifest, SourceFiles, err := UnpackSourceZip(data)
	if err != nil {
		return nil, err
	}
	if manifest.Command != "install" && manifest.Command != "build" {
		return nil, ErrNotSupportCommand
	}

	binary, err := Build(manifest, SourceFiles)
	if err != nil {
		return nil, err
	}
	return binary, nil
}

func UnpackSourceZip(data []byte) (*common.Manifest, []*common.SourceFile, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, err
	}

	var manifest common.Manifest
	SourceFiles := make([]*common.SourceFile, 0, len(zr.File)-1)
	for _, file := range zr.File {
		if file.Name == "manifest.json" {
			fr, err := file.Open()
			if err != nil {
				return nil, nil, err
			}
			err = json.NewDecoder(fr).Decode(&manifest)
			if err != nil {
				return nil, nil, err
			}
		} else {
			fr, err := file.Open()
			if err != nil {
				return nil, nil, err
			}
			sf := &common.SourceFile{
				Path:       file.Name,
				ReadCloser: fr,
			}
			SourceFiles = append(SourceFiles, sf)
		}
	}
	return &manifest, SourceFiles, nil
}

func Build(manifest *common.Manifest, SourceFiles []*common.SourceFile) ([]byte, error) {
	tempDir, err := ioutil.TempDir("", "devfarm_builder")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)

	dirPaths := make([]string, 0, len(SourceFiles))
	for _, sf := range SourceFiles {
		dir := filepath.Dir(sf.Path)
		dirPaths = append(dirPaths, dir)
	}

	sort.Strings(dirPaths)

	nodeMarker := make([]bool, len(dirPaths))
	for i := 1; i < len(dirPaths); i++ {
		if strings.HasPrefix(dirPaths[i]+"/", dirPaths[i-1]+"/") {
			nodeMarker[i-1] = true
		}
	}
	for i, v := range dirPaths {
		if !nodeMarker[i] {
			err := os.MkdirAll(fmt.Sprintf(`%s/%s`, tempDir, v), os.ModeDir)
			if err != nil {
				return nil, err
			}
		}
	}

	for _, v := range SourceFiles {
		err := func(sf *common.SourceFile) error {
			if strings.HasPrefix(sf.Path, "src/") {
				file, err := os.Create(fmt.Sprintf(`%s/%s`, tempDir, sf.Path))
				if err != nil {
					return err
				}
				defer file.Close()
				_, err = io.Copy(file, sf.ReadCloser)
				if err != nil {
					return err
				}
			}
			return nil
		}(v)
		if err != nil {
			return nil, err
		}
	}

	gobin := filepath.Clean(fmt.Sprintf(`%s%s`, os.Getenv("GOROOT"), `/bin/go`))
	Args := []string{manifest.Command}
	Args = append(Args, manifest.BuildFlags...)
	Args = append(Args, manifest.Packages)

	cmd := exec.Command(gobin, Args...)
	cmd.Dir = tempDir

	envs := make([]string, 0)
	for _, v := range os.Environ() {
		if strings.HasPrefix(v, "GOPATH=") {
			envs = append(envs, "GOPATH="+tempDir)
		} else {
			envs = append(envs, v)
		}
	}
	cmd.Env = envs

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		if !strings.Contains(err.Error(), "exit status") {
			return nil, err
		}
	}

	msg := stderr.String() + stdout.String()
	if len(msg) > 0 {
		if strings.Contains(msg, ":") {
			return nil, errors.New(msg)
		}
	}

	var buffer bytes.Buffer
	zw := zip.NewWriter(&buffer)
	err = utils.AddDirToZip(zw, fmt.Sprintf(`%s/bin/`, tempDir), "", nil)
	if err != nil {
		return nil, err
	}

	for _, v := range SourceFiles {
		err := func(sf *common.SourceFile) error {
			if !strings.HasPrefix(sf.Path, "src/") {
				fw, err := zw.Create(sf.Path)
				if err != nil {
					return err
				}
				_, err = io.Copy(fw, sf.ReadCloser)
				if err != nil {
					return err
				}
			}
			return nil
		}(v)
		if err != nil {
			return nil, err
		}
	}
	zw.Close()
	return buffer.Bytes(), nil
}
