// Copyright 2020 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sink

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
)

// DefaultMaxInMemoryFileSize is the default value for ArtifactUploader.MaxInMemoryFileSize.
const DefaultMaxInMemoryFileSize = 64 * 1024

// ArtifactUploader provides functions for uploading artifacts to ResultDB.
type ArtifactUploader struct {
	// Client is an HTTP client used for uploading artifacts to ResultDB.
	Client *http.Client
	// Host is the host of a ResultDB instance to upload artifacts to.
	Host string

	// MaxInMemoryFileSize is the maximum size of an artifact file that can be loaded into
	// memory.
	//
	// ArtifactUploader reads the entire contents of a file twice; one read for calculating
	// the hash and another read for transmitting the contents over network. If the size
	// is equal to or smaller than MaxInMemoryFileSize, ArtifactUploader loads the entire
	// contents into a memory buffer to minimize disk I/O.
	//
	// If this field is set with 0, DefaultMaxInMemoryFileSize is used.
	MaxInMemoryFileSize int64
}

func (u *ArtifactUploader) newRequest(ctx context.Context, name, contentType string, input io.ReadSeeker, updateToken string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "PUT", fmt.Sprintf("https://%s/%s", u.Host, name), input)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create a request").Err()
	}
	hash, err := calculateHash(input)
	if err != nil {
		return nil, errors.Annotate(err, "failed to calcualte hash").Err()
	}
	// rewind the current position back to the beginning so that the request body can be
	// re-read by HTTPClient.Do.
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		return nil, errors.Annotate(err, "failed to reset the stream cursor").Err()
	}

	req.Header.Add("Content-Hash", hash)
	if contentType != "" {
		req.Header.Add("Content-Type", contentType)
	}
	req.Header.Add("Update-Token", updateToken)
	return req, nil
}

// Upload uploads an artifact from a given slice of bytes.
//
// `name` is the artifact name, which uniquely identifies the artifact globally. It must
// conform to the format described in the name property of message Artifact at
// https://chromium.googlesource.com/infra/luci/luci-go/+/master/resultdb/proto/v1/artifact.proto
//
// go.chromium.org/luci/resultdb/pbutil package provides utility functions to generate
// an Artifact name for test-result-level and invocation-level artifacts.
// - https://pkg.go.dev/go.chromium.org/luci/resultdb/pbutil?tab=doc#InvocationArtifactName
// - https://pkg.go.dev/go.chromium.org/luci/resultdb/pbutil?tab=doc#TestResultArtifactName
func (u *ArtifactUploader) Upload(ctx context.Context, name, contentType string, contents []byte, updateToken string) error {
	req, err := u.newRequest(ctx, name, contentType, bytes.NewReader(contents), updateToken)
	if err != nil {
		return err
	}
	return u.send(req)
}

// UploadFromFile uploads an artifact from a given file path.
//
// `name` is the artifact name, which uniquely identifies the artifact globally. It must
// conform to the format described in the name property of message Artifact at
// https://chromium.googlesource.com/infra/luci/luci-go/+/master/resultdb/proto/v1/artifact.proto
func (u *ArtifactUploader) UploadFromFile(ctx context.Context, name, contentType, path, updateToken string) error {
	st, err := os.Stat(path)
	if err != nil {
		return errors.Annotate(err, "failed to query the file status").Err()
	}
	// small file?
	smallFS := u.MaxInMemoryFileSize
	if smallFS == 0 {
		smallFS = DefaultMaxInMemoryFileSize
	}
	if st.Size() <= smallFS {
		contents, err := ioutil.ReadFile(path)
		if err != nil {
			return errors.Annotate(err, "failed to read file contents").Err()
		}
		return u.Upload(ctx, name, contentType, contents, updateToken)
	}

	input, err := os.Open(path)
	if err != nil {
		return errors.Annotate(err, "failed to open file").Err()
	}
	defer input.Close()
	req, err := u.newRequest(ctx, name, contentType, input, updateToken)
	if err != nil {
		return err
	}
	// Client.Do closes the body on 3xx responses and GetBody needs to reopen the file.
	// This function wraps the file object with NopCloser and closes the body on return, so
	// that GetBody can simply move the stream cursor to the beginning without reopening
	// the file object.
	req.Body = ioutil.NopCloser(req.Body)
	req.GetBody = func() (io.ReadCloser, error) {
		if _, err := input.Seek(0, io.SeekStart); err != nil {
			return nil, errors.Annotate(err, "failed to reset the stream cursor").Err()
		}
		return ioutil.NopCloser(input), nil
	}
	req.ContentLength = st.Size()
	return u.send(req)
}

func (u *ArtifactUploader) send(req *http.Request) error {
	return retry.Retry(req.Context(), transient.Only(retry.Default), func() error {
		resp, err := u.Client.Do(req)
		if err != nil {
			return errors.Annotate(err, "failed to send HTTP request").Err()
		}

		code := resp.StatusCode
		// ResultDB returns StatusNoContent on success.
		if code == http.StatusNoContent {
			return nil
		}

		// Tag the error as an transient error, if retriable.
		hErr := errors.Reason("http request failed(%d): %s", resp.StatusCode, resp.Status)
		if code == http.StatusRequestTimeout || code == http.StatusTooManyRequests || code >= 500 {
			hErr = hErr.Tag(transient.Tag)
		}
		return hErr.Err()
	}, nil)
}

func calculateHash(input io.Reader) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, input); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}
