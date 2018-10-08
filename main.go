package main

import (
	"bytes"
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
	"sync"

	"github.com/grafov/m3u8"
)

// FTP protocol
const FTP = "ftp"

// S3 protocol
const S3 = "s3"

var (
	// Version is the current version of the tool, added on build time
	Version string
	// Build is the date of the build, added on build time
	Build string
)

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

func download(url string, fs fileSystem) (string, error) {

	body, err := fetch(url)

	if err != nil {
		return "", fmt.Errorf("could not download segment %s: %v", url, err)
	}

	defer body.Close()

	fileName := path.Base(url)
	out, err := fs.WriteFrom(body, fileName)

	if err != nil {
		return "", fmt.Errorf("could not write segment %s: %v", url, err)
	}

	return out, nil
}

func downloadPlaylist(u string, fs fileSystem, downloader chan string, stopped chan bool) error {
	playlistURL, err := url.Parse(u)

	if err != nil {
		return fmt.Errorf("url is not valid: %v", err)
	}

	playlistBody, err := fetch(playlistURL.String())

	if err != nil {
		return fmt.Errorf("could not fetch playlist: %v", err)
	}

	content, err := ioutil.ReadAll(playlistBody)

	playlistBody.Close()

	if err != nil {
		return fmt.Errorf("could not read all content: %v", err)
	}

	playlist, listType, err := m3u8.Decode(*bytes.NewBuffer(content), true)

	if err != nil {
		return fmt.Errorf("could not parse m3u8 playlist: %v", err)
	}

	switch listType {
	case m3u8.MEDIA:
		mediapl := playlist.(*m3u8.MediaPlaylist)

		fileName := path.Base(playlistURL.Path)
		_, err := fs.Write(content, fileName)

		if err != nil {
			return fmt.Errorf("could not write playlist %s %v", fileName, err)
		}

		log.Printf("Downloaded playlist %s\n", fileName)

		for _, segment := range mediapl.Segments {
			select {
			case <-stopped:
				return nil
			default:
			}

			if segment != nil {
				downloader <- strings.Replace(playlistURL.String(), fileName, segment.URI, -1)
			}
		}

	case m3u8.MASTER:
		masterpl := playlist.(*m3u8.MasterPlaylist)

		fileName := path.Base(playlistURL.Path)
		_, err := fs.Write(content, fileName)

		if err != nil {
			return fmt.Errorf("could not write master playlist %s %v", fileName, err)
		}

		log.Printf("Downloaded master %s\n", fileName)

		var subPlaylistURL string

		for _, variant := range masterpl.Variants {
			if variant != nil {
				subPlaylistURL = strings.Replace(playlistURL.String(), fileName, variant.URI, -1)

				err := downloadPlaylist(subPlaylistURL, fs, downloader, stopped)

				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func downloadStream(u string, fs fileSystem, workers int) error {

	// Channel to signal successfuly completion
	done := make(chan bool)
	// Channel to signal interruption
	stopped := make(chan bool)
	// Channel to send errors
	errors := make(chan error)

	// To know when all workers finished
	// so all work is processed
	var wg sync.WaitGroup
	wg.Add(workers)

	go func() {
		wg.Wait()
		close(done)
	}()

	downloader := make(chan string)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for url := range downloader {
				out, err := download(url, fs)

				select {
				case <-stopped:
					return // Error somewhere, terminate
				default: // Default is must to avoid blocking
				}

				if err != nil {
					errors <- err
					return
				}

				log.Printf("Downloaded: %s", out)
			}
		}()
	}

	downloadPlaylist(u, fs, downloader, stopped)
	close(downloader)

	for {
		select {
		case <-done:
			close(stopped)
			return nil
		case err := <-errors:
			// cancel() may be called multiple times
			close(stopped)
			return err
		}
	}
}

func main() {
	fmt.Printf("HLS Downloader. Version: %s (%s)\n", Version, Build)

	input := flag.String("i", "", "Manifest (m3u8) url to download")
	output := flag.String("o", "", "Path or URI where the files will be stored (local path or S3 bucket in the format s3://<bucket>/<path>")
	workers := flag.Int("w", 3, "Number of workers to execute concurrent operations. Min: 1, Max: 10")

	flag.Parse()

	if len(*input) == 0 {
		flag.Usage()
		log.Fatal("ERROR: input (-i) must be defined")
	}

	// Use current directory if none provided
	if len(*output) == 0 {
		ex, err := os.Executable()

		if err != nil {
			panic(err)
		}

		*output = filepath.Dir(ex)
	}

	var fs fileSystem
	var err error
	delimiter := strings.Index(*output, "://")

	if delimiter > -1 {
		protocol := (*output)[0:delimiter]

		switch protocol {
		case S3:
			uriParts := strings.Split((*output)[delimiter+3:], "/")
			path := ""
			bucket := uriParts[0]

			if len(uriParts) > 1 {
				path = strings.Join(uriParts[1:], "/")
			}

			log.Printf("Using output as S3 bucket %s, path %s", bucket, path)

			fs, err = NewS3FS(bucket, path)

			if err != nil {
				log.Fatal(err)
			}
		default:
			log.Fatalf("Protocol not supported: %s", protocol)
		}
	} else {
		if _, err = os.Stat(*output); err != nil {
			log.Fatalf("Output dir does not exists %s", *output)
		}

		log.Printf("Using output as local dir %s\n", *output)

		fs = &localFS{*output}
	}

	if *workers < 1 {
		*workers = 1
	}

	if *workers > 10 {
		*workers = 10
	}

	err = downloadStream(*input, fs, *workers)

	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Successfuly downloaded %s \n", *input)
}
