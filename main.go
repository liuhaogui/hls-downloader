package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/grafov/m3u8"
)

type fileSystem interface {
	Write(content []byte, fileName string) (string, error)
	WriteFrom(stream io.Reader, fileName string) (string, error)
}

type localFS struct {
	localDir string
}

type S3FS struct {
	config map[string]string
}

func (fs *localFS) WriteFrom(stream io.Reader, fileName string) (string, error) {
	if len(fs.localDir) == 0 {
		return "", errors.New("local directory is not defined")
	}

	out, err := os.Create(path.Join(fs.localDir, fileName))

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

	out, err := os.Create(path.Join(fs.localDir, fileName))

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

func fetch(url string) (io.ReadCloser, error) {
	log.Printf("Getting %s \n", url)

	res, err := http.Get(url)

	if err != nil {
		return nil, fmt.Errorf("could not get: %s: %v", url, err)
	}

	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()

		// TODO: wait instead of throwing error, add timeout or max wait
		if res.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("rate limit reached")
		}

		return nil, fmt.Errorf("bad response from server: %s", res.Status)
	}

	return res.Body, nil
}

func downloadPlaylist(u string, fs fileSystem) error {
	playlistURL, err := url.Parse(u)

	if err != nil {
		log.Fatal(err)
	}

	playlistBody, err := fetch(playlistURL.String())

	if err != nil {
		log.Fatal(err)
	}

	content, err := ioutil.ReadAll(playlistBody)

	playlistBody.Close()

	if err != nil {
		return fmt.Errorf("could not read all content: %v", err)
	}

	playlist, listType, err := m3u8.Decode(*bytes.NewBuffer(content), true)

	if err != nil {
		log.Fatal(err)
	}

	switch listType {
	case m3u8.MEDIA:
		mediapl := playlist.(*m3u8.MediaPlaylist)

		fileName := path.Base(playlistURL.Path)
		fs.Write(content, fileName)

		var segmentUrl string

		for k, segment := range mediapl.Segments {
			if segment != nil {
				segmentUrl = strings.Replace(playlistURL.String(), fileName, segment.URI, -1)

				segmentBody, err := fetch(segmentUrl)

				if err != nil {
					return fmt.Errorf("could not download segment %d- %s: %v", k, segmentUrl, err)
				}

				fileName = path.Base(segment.URI)
				fs.WriteFrom(segmentBody, fileName)

				segmentBody.Close()
			}
		}
	case m3u8.MASTER:
		masterpl := playlist.(*m3u8.MasterPlaylist)

		fileName := path.Base(playlistURL.Path)
		fs.Write(content, fileName)

		var subPlaylistUrl string

		for k, variant := range masterpl.Variants {
			if variant != nil {
				subPlaylistUrl = strings.Replace(playlistURL.String(), fileName, variant.URI, -1)

				log.Printf("Downloading sub playlist %d- %s\n", k, variant.URI)
				downloadPlaylist(subPlaylistUrl, fs)
			}
		}
	}

	return nil
}

func main() {
	input := flag.String("i", "", "Manifest (m3u8) url to download")
	output := flag.String("o", "", "Path or URI where the files will be stored")
	flag.Parse()

	// Use current directory if none provided
	if len(*output) == 0 {
		ex, err := os.Executable()

		if err != nil {
			panic(err)
		}

		*output = filepath.Dir(ex)

		log.Printf("Using output dir as %s\n", *output)
	}

	// TODO: Allow S3 FS
	fs := &localFS{*output}

	// TODO: Add the gophers fun (use concurrency)
	downloadPlaylist(*input, fs)

	log.Printf("Successfuly downloaded %s \n", *input)
}
