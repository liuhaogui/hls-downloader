package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
)

type localFS struct {
	localDir string
}

func (fs *localFS) WriteFrom(stream io.Reader, fileName string) (string, error) {
	if len(fs.localDir) == 0 {
		return "", errors.New("local directory is not defined")
	}

	fullPath := path.Join(fs.localDir, fileName)
	path := path.Dir(fullPath)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Println("Creating path " + path)
		err := os.MkdirAll(path, os.ModePerm)

		if err != nil {
			return "", fmt.Errorf("path to file could not be created: %v", err)
		}
	}

	out, err := os.Create(fullPath)

	if err != nil {
		return "", fmt.Errorf("could not create local file: %v", err)
	}

	defer out.Close()

	_, err = io.Copy(out, stream)

	if err != nil {
		return "", fmt.Errorf("could not write stream to local file: %v", err)
	}

	return out.Name(), nil
}

func (fs *localFS) Write(content []byte, fileName string) (string, error) {
	if len(fs.localDir) == 0 {
		return "", errors.New("local directory is not defined")
	}

	// fileName could contain parent folders
	fullPath := path.Join(fs.localDir, fileName)
	path := path.Dir(fullPath)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Println("Creating path " + path)
		err := os.MkdirAll(path, os.ModePerm)

		if err != nil {
			return "", fmt.Errorf("path to file could not be created: %v", err)
		}
	}

	out, err := os.Create(fullPath)

	if err != nil {
		return "", fmt.Errorf("could not create local file: %v", err)
	}

	defer out.Close()

	_, err = out.Write(content)

	if err != nil {
		return "", fmt.Errorf("could not write content to local file: %v", err)
	}

	return out.Name(), nil
}
