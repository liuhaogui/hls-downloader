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

	"github.com/grafov/m3u8"
)

// FTP protocol
const FTP = "ftp"

// S3 protocol
const S3 = "s3"

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

func downloadPlaylist(u string, fs fileSystem) (bool, error) {
	playlistURL, err := url.Parse(u)

	if err != nil {
		log.Fatal(err)
	}

	playlistBody, err := fetch(playlistURL.String())

	if err != nil {
		return false, fmt.Errorf("could not fetch playlist: %v", err)
	}

	content, err := ioutil.ReadAll(playlistBody)

	playlistBody.Close()

	if err != nil {
		return false, fmt.Errorf("could not read all content: %v", err)
	}

	playlist, listType, err := m3u8.Decode(*bytes.NewBuffer(content), true)

	if err != nil {
		log.Fatal(err)
	}

	switch listType {
	case m3u8.MEDIA:
		mediapl := playlist.(*m3u8.MediaPlaylist)

		fileName := path.Base(playlistURL.Path)
		_, err := fs.Write(content, fileName)

		if err != nil {
			return false, fmt.Errorf("could not write sub playlist %s %v", fileName, err)
		}

		var segmentUrl string

		for k, segment := range mediapl.Segments {
			if segment != nil {
				segmentUrl = strings.Replace(playlistURL.String(), fileName, segment.URI, -1)

				segmentBody, err := fetch(segmentUrl)

				if err != nil {
					return false, fmt.Errorf("could not download segment %d - %s: %v", k, segmentUrl, err)
				}

				fileName = path.Base(segment.URI)
				_, err = fs.WriteFrom(segmentBody, fileName)

				segmentBody.Close()

				if err != nil {
					return false, fmt.Errorf("could not write segment %d - %s: %v", k, segmentUrl, err)
				}
			}
		}
	case m3u8.MASTER:
		masterpl := playlist.(*m3u8.MasterPlaylist)

		fileName := path.Base(playlistURL.Path)
		_, err := fs.Write(content, fileName)

		if err != nil {
			return false, fmt.Errorf("could not write master playlist %s %v", fileName, err)
		}

		var subPlaylistUrl string

		for k, variant := range masterpl.Variants {
			if variant != nil {
				subPlaylistUrl = strings.Replace(playlistURL.String(), fileName, variant.URI, -1)

				log.Printf("Downloading sub playlist %d- %s\n", k, variant.URI)

				_, err := downloadPlaylist(subPlaylistUrl, fs)

				if err != nil {
					return false, err
				}
			}
		}
	}

	return false, nil
}

func main() {
	input := flag.String("i", "", "Manifest (m3u8) url to download")
	output := flag.String("o", "", "Path or URI where the files will be stored (local path or S3 bucket in the format s3://<bucket>/<path>")

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

	// TODO: Add the gophers fun (use concurrency)
	_, err = downloadPlaylist(*input, fs)

	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Successfuly downloaded %s \n", *input)
}
