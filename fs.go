package main

import "io"

type fileSystem interface {
	Write(content []byte, fileName string) (string, error)
	WriteFrom(stream io.Reader, fileName string) (string, error)
}
