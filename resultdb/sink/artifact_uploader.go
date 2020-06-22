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

// MaxInMemoryFileSize is the maximum size of an artifact file that can be loaded into
// memory.
//
// ArtifactUploader reads the entire contents of a file twice; one for calculating the hash
// and one for transmitting the contents over network. If the size is equal to or smaller
// than MaxInMemoryFileSize, ArtifactUploader dumps the entire contents into a memory buffer
// to minimize disk I/O.
const MaxInMemoryFileSize = 64 * 1024

// ArtifactUploader provides functions for uploading artifacts to ResultDB.
type ArtifactUploader struct {
	// Client is an HTTP client used for uploading artifacts to ResultDB.
	Client *http.Client
	// Host is the host of a ResultDB instance to upload artifacts to.
	Host string
}

// NewArtifactUploader returns a new ArtifactUploader.
func NewArtifactUploader(c *http.Client, host string) *ArtifactUploader {
	return &ArtifactUploader{Client: c, Host: host}
}

// Upload uploads an artifact from a given input stream.
//
// `name` is the artifact name, which uniquely identifies the artifact globally. It must
// conform to the format described in the name property of message Artifact at
// https://chromium.googlesource.com/infra/luci/luci-go/+/master/resultdb/proto/v1/artifact.proto
//
// If the input source is a file, it's highly recommended to use UploadFromFile, instead.
func (u *ArtifactUploader) Upload(ctx context.Context, name, contentType string, input io.ReadSeeker, updateToken string) error {
	req, err := http.NewRequestWithContext(ctx, "PUT", fmt.Sprintf("https://%s/%s", u.Host, name), input)
	if err != nil {
		return err
	}
	hash, err := calculateHash(input)
	if err != nil {
		return err
	}
	// rewind the current position back to the beginning so that the request body can be
	// re-read by HTTPClient.Do.
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		return err
	}

	req.Header.Add("Content-Hash", hash)
	if contentType != "" {
		req.Header.Add("Content-Type", contentType)
	}
	req.Header.Add("Update-Token", updateToken)
	return u.send(ctx, req)
}

// Upload uploads an artifact from a given input stream.
//
// `name` is the artifact name, which uniquely identifies the artifact globally. It must
// conform to the format described in the name property of message Artifact at
// https://chromium.googlesource.com/infra/luci/luci-go/+/master/resultdb/proto/v1/artifact.proto
func (u *ArtifactUploader) UploadFromFile(ctx context.Context, name, contentType, path, updateToken string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	// if the file is small enough to be loaded in memory, load it with
	// bytes.NewBuffer() to avoid unnecessary file operations.
	if st.Size() <= MaxInMemoryFileSize {
		content, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		return u.Upload(ctx, name, contentType, bytes.NewReader(content), updateToken)
	}

	input, err := os.Open(path)
	if err != nil {
		return err
	}
	hash, err := calculateHash(input)
	if err != nil {
		return err
	}
	// rewind the current position back to the beginning so that the request body can be
	// re-read by HTTPClient.Do.
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", fmt.Sprintf("https://%s/%s", u.Host, name), input)
	if err != nil {
		return err
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return os.Open(path)
	}
	req.ContentLength = st.Size()
	req.Header.Add("Content-Hash", hash)
	if contentType != "" {
		req.Header.Add("Content-Type", contentType)
	}
	req.Header.Add("Update-Token", updateToken)
	return u.send(ctx, req)
}

func (u *ArtifactUploader) send(ctx context.Context, req *http.Request) error {
	return retry.Retry(ctx, transient.Only(retry.Default), func() error {
		resp, err := u.Client.Do(req)
		if err != nil {
			return err
		}

		code := resp.StatusCode
		// ResultDB returns StatusNoContent on success.
		if code == http.StatusNoContent {
			return nil
		}

		// Tag the error as an transient error, if retriable.
		hErr := errors.Reason("http request failed(%d): %s", resp.StatusCode, resp.Status)
		if code == http.StatusRequestTimeout || code == http.StatusTooManyRequests {
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
