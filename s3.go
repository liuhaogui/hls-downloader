package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type S3FS struct {
	session *session.Session
	bucket  string
	path    string
}

// NewS3FS creates a file system instance for given S3 bucket
func NewS3FS(bucket, path string) (fileSystem, error) {
	sess, err := session.NewSession()

	if err != nil {
		return nil, fmt.Errorf("could not create AWS Session: %v", err)
	}

	return &S3FS{
		session: sess,
		bucket:  bucket,
		path:    path,
	}, nil
}

func (fs *S3FS) validate() (bool, error) {
	if fs.session == nil {
		return false, errors.New("AWS session cannot be nil")
	}

	if len(fs.bucket) == 0 {
		return false, errors.New("bucket is not defined")
	}

	return true, nil
}

func (fs *S3FS) WriteFrom(stream io.Reader, fileName string) (string, error) {
	if ok, err := fs.validate(); !ok {
		return "", err
	}

	fileName = path.Join(fs.path, fileName)

	uploader := s3manager.NewUploader(fs.session)

	out, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(fs.bucket),
		Key:    aws.String(fileName),
		Body:   stream,
	})

	if err != nil {
		return "", fmt.Errorf("could not upload stream to S3 bucket: %v", err)
	}

	return out.Location, nil
}

func (fs *S3FS) Write(content []byte, fileName string) (string, error) {
	if ok, err := fs.validate(); !ok {
		return "", err
	}

	fileName = path.Join(fs.path, fileName)

	uploader := s3manager.NewUploader(fs.session)

	out, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(fs.bucket),
		Key:    aws.String(fileName),
		Body:   bytes.NewReader(content),
	})

	if err != nil {
		return "", fmt.Errorf("could not upload stream to S3 bucket: %v", err)
	}

	return out.Location, nil
}
