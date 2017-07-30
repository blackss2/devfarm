package runner

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/blackss2/devfarm/common"
)

func RunFromBinaryZip(ctx context.Context, data []byte, inChan io.Reader, outChan io.Writer, errChan io.Writer, portChan io.Writer) error {
	BinaryFiles, err := UnpackBinaryZip(data)
	if err != nil {
		return err
	}

	err = RunBinary(ctx, BinaryFiles, inChan, outChan, errChan, portChan)
	if err != nil {
		return err
	}
	return nil
}

func UnpackBinaryZip(data []byte) ([]*common.BinaryFile, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}

	BinaryFiles := make([]*common.BinaryFile, 0, len(zr.File)-1)
	for _, file := range zr.File {
		fr, err := file.Open()
		if err != nil {
			return nil, err
		}
		bf := &common.BinaryFile{
			Path:       file.Name,
			ReadCloser: fr,
		}
		BinaryFiles = append(BinaryFiles, bf)
	}
	return BinaryFiles, nil
}

func RunBinary(ctx context.Context, BinaryFiles []*common.BinaryFile, inChan io.Reader, outChan io.Writer, errChan io.Writer, portChan io.Writer) error {
	tempDir, err := ioutil.TempDir("", "devfarm_runner")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	dirPaths := make([]string, 0, len(BinaryFiles))
	for _, bf := range BinaryFiles {
		dir := filepath.Dir(bf.Path)
		dirPaths = append(dirPaths, dir)
	}

	sort.Strings(dirPaths)

	nodeMarker := make([]bool, len(dirPaths))
	for i := 1; i < len(dirPaths); i++ {
		if strings.HasPrefix(dirPaths[i], dirPaths[i-1]) {
			nodeMarker[i-1] = true
		}
	}
	for i, v := range dirPaths {
		if !nodeMarker[i] {
			err := os.MkdirAll(fmt.Sprintf(`%s/%s`, tempDir, v), os.ModeDir)
			if err != nil {
				return err
			}
		}
	}

	var binFile string
	hasResource := false
	for _, v := range BinaryFiles {
		err := func(bf *common.BinaryFile) error {
			path := fmt.Sprintf(`%s/%s`, tempDir, bf.Path)
			file, err := os.Create(path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(file, bf.ReadCloser)
			if err != nil {
				return err
			}

			if !strings.HasPrefix(bf.Path, "__resources") {
				ext := filepath.Ext(bf.Path)
				if ext != ".so" {
					binFile = bf.Path
				}

				if runtime.GOOS != "windows" {
					cmd := exec.Command("chmod", "+x", path)
					cmd.Dir = tempDir
					err = cmd.Run()
					if err != nil {
						return err
					}
				}
			} else {
				hasResource = true
			}
			return nil
		}(v)
		if err != nil {
			return err
		}
	}

	for i, v := range dirPaths {
		if !nodeMarker[i] {
			err := os.MkdirAll(fmt.Sprintf(`%s/%s`, tempDir, v), os.ModeDir)
			if err != nil {
				return err
			}
		}
	}

	if !hasResource {
		err := os.MkdirAll(fmt.Sprintf(`%s/__resources`, tempDir), os.ModeDir)
		if err != nil {
			return err
		}
	}

	runbin := tempDir + "/" + binFile
	Args := []string{}
	cmd := exec.CommandContext(ctx, runbin, Args...)
	cmd.Dir = tempDir + "/__resources"

	envs := make([]string, 0)
	for _, v := range os.Environ() {
		envs = append(envs, v)
	}
	cmd.Env = envs

	cmd.Stdin = inChan
	cmd.Stdout = outChan
	cmd.Stderr = errChan

	err = cmd.Start()
	if err != nil {
		return err
	}

	if runtime.GOOS != "windows" {
		go func() error {
			for {
				time.Sleep(time.Second * 3)
				grep, err := exec.Command("/bin/sh", "-c", fmt.Sprintf(`ls -l /proc/%d/fd | grep socket`, cmd.Process.Pid)).CombinedOutput()
				if err != nil {
					if !strings.Contains(err.Error(), "exit status") {
						continue
					}
				}

				ls := strings.Split(string(grep), "\n")
				ports := make([]int64, 0, len(ls))
				for _, v := range ls {
					idx := strings.Index(v, "socket:[")
					if idx >= 0 {
						inode := v[idx+8 : len(v)-1]

						cat, err := exec.Command("/bin/sh", "-c", fmt.Sprintf(`cat /proc/net/tcp6 | grep %s`, inode)).CombinedOutput()
						if err != nil {
							if !strings.Contains(err.Error(), "exit status") {
								continue
							}
						}
						if len(cat) > 0 {
							vs := strings.Split(strings.TrimSpace(string(cat)), " ")
							if len(vs) > 1 {
								ps := strings.Split(vs[1], ":")
								if len(ps) > 1 {
									phex := ps[1]
									port, err := strconv.ParseInt(phex, 16, 32)
									if err != nil {
										continue
									}
									ports = append(ports, port)
								}
							}
						}
					}
				}
				//UpdatePortMapper(ports)
				sports := make([]string, 0, len(ports))
				for _, v := range ports {
					port := strconv.Itoa(int(v))
					sports = append(sports, port)
				}
				portChan.Write([]byte(strings.Join(sports, ",")))
			}
		}()
	}

	_, err = cmd.Process.Wait()
	if err != nil {
		return err
	}
	return nil
}
