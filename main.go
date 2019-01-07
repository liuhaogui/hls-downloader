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
	"sync/atomic"

	"github.com/grafov/m3u8"
)

// FTP protocol
const FTP = "ftp"

// S3 protocol
const S3 = "s3"

type downloadInfo struct {
	url      string
	destPath string
}

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

func download(url, destPath string, fs fileSystem) (string, error) {

	if url == "" || destPath == "" {
		return "", fmt.Errorf("arguments cannot be empty")
	}

	body, err := fetch(url)

	if err != nil {
		return "", fmt.Errorf("could not download segment %s: %v", url, err)
	}

	defer body.Close()

	out, err := fs.WriteFrom(body, destPath)

	if err != nil {
		return "", fmt.Errorf("could not write segment %s: %v", url, err)
	}

	return out, nil
}

func downloadPlaylist(u string, fs fileSystem, downloader chan downloadInfo, stopped chan bool, errors chan error) {
	playlistURL, err := url.Parse(u)

	if err != nil {
		errors <- fmt.Errorf("url is not valid: %v", err)
		return
	}

	playlistBody, err := fetch(playlistURL.String())

	if err != nil {
		errors <- fmt.Errorf("could not fetch playlist: %v", err)
		return
	}

	content, err := ioutil.ReadAll(playlistBody)

	playlistBody.Close()

	if err != nil {
		errors <- fmt.Errorf("could not read all content: %v", err)
		return
	}

	playlist, listType, err := m3u8.Decode(*bytes.NewBuffer(content), true)

	if err != nil {
		errors <- fmt.Errorf("could not parse m3u8 playlist: %v", err)
		return
	}

	process := make(chan bool)

	switch listType {
	case m3u8.MEDIA:
		mediapl := playlist.(*m3u8.MediaPlaylist)

		fileName := path.Base(playlistURL.Path)
		urlPrefix := strings.TrimSuffix(playlistURL.String(), fileName)

		var segment *m3u8.MediaSegment

		// Trim possible absolute url to each segment
		// to make the stream playable from new location
		for _, segment = range mediapl.Segments {
			if segment != nil {
				segment.URI = strings.TrimPrefix(segment.URI, urlPrefix)
			}
		}

		_, err := fs.Write([]byte(mediapl.String()), fileName)

		if err != nil {
			errors <- fmt.Errorf("could not write playlist %s %v", fileName, err)
			return
		}

		log.Printf("Downloaded playlist %s\n", fileName)

		for _, segment = range mediapl.Segments {
			select {
			case <-stopped:
				return
			default:
			}

			if segment != nil {
				info := downloadInfo{}
				_, err = url.ParseRequestURI(segment.URI)

				if err != nil {
					info.url = strings.Replace(playlistURL.String(), fileName, segment.URI, -1)
					info.destPath = segment.URI
				} else {
					info.url = segment.URI
					info.destPath = path.Base(segment.URI)
				}

				downloader <- info
			}
		}

	case m3u8.MASTER:
		masterpl := playlist.(*m3u8.MasterPlaylist)

		// Trim possible absolute url to each sub playlist
		// to make the stream playable from new location
		for _, variant := range masterpl.Variants {
			variant.URI = path.Base(variant.URI)
		}

		fileName := path.Base(playlistURL.Path)

		_, err := fs.Write([]byte(masterpl.String()), fileName)

		if err != nil {
			errors <- fmt.Errorf("could not write master playlist %s %v", fileName, err)
			return
		}

		log.Printf("Downloaded master %s\n", fileName)

		var subPlaylistURL string
		length := len(masterpl.Variants)

		log.Printf("%d sub-playlists found", length)

		for _, variant := range masterpl.Variants {
			select {
			case <-stopped:
				return
			default:
			}

			if variant != nil {
				// As for now this won't be an url never
				// since we are trimming it above
				// TODO: make trimming URI optional or just remove url check
				_, err = url.ParseRequestURI(variant.URI)
				if err != nil {
					subPlaylistURL = strings.Replace(playlistURL.String(), fileName, variant.URI, -1)
				} else {
					subPlaylistURL = variant.URI
				}

				// Closures in Go are lexically scoped.
				// ... any variables referenced within the closure from the "outer" scope
				// are not a copy but are in fact a reference
				go func(subPlaylistURL string) {
					defer func() {
						process <- true
					}()
					log.Printf("Downloading sub playlist %s", subPlaylistURL)
					downloadPlaylist(subPlaylistURL, fs, downloader, stopped, errors)
				}(subPlaylistURL)
			} else {
				process <- true
			}
		}

		for i := 1; i <= length; i++ {
			<-process
			log.Printf("Subplaylist processing finished")
		}
	}
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

	downloader := make(chan downloadInfo)

	var count uint64
	var total uint64

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for info := range downloader {
				out, err := download(info.url, info.destPath, fs)

				if err != nil {
					errors <- err
					return
				}

				atomic.AddUint64(&count, 1)

				select {
				case <-stopped:
					return // Error somewhere, terminate
				default: // Default is must to avoid blocking
				}

				total = count // TODO: find actual total

				log.Printf("%s (%d / %d)", out, count, total)
			}
		}()
	}

	go func() {
		downloadPlaylist(u, fs, downloader, stopped, errors)
		close(downloader)
	}()

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
