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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"
)

// HTTPClient is an interface that wraps the Do method of http.Client.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type ArtifactUploader struct {
	// Client is an HTTP client used for uploading artifacts to ResultDB.
	Client HTTPClient
	// Host is the host of a ResultDB instance to upload artifacts to.
	Host string
}

// NewArtifactUploader returns a new ArtifactUploader.
func NewArtifactUploader(c HTTPClient, host string) *ArtifactUploader {
	return &ArtifactUploader{Client: c, Host: host}
}

// NewRequest returns a new HTTP request with the artifact in body.
func (u *ArtifactUploader) NewRequest(name string, art *sinkpb.Artifact, token string) (*http.Request, error) {
	req, err := http.NewRequest("PUT", fmt.Sprintf("https://%s/%s", u.Host, name), nil)
	if err != nil {
		return nil, err
	}

	// content-length
	input, size, err := openArtifact(art)
	if err != nil {
		return nil, err
	}
	req.Body = input
	req.ContentLength = size

	// content-hash
	hash, err := calculateHash(input)
	if err != nil {
		return nil, err
	}
	// rewind the current position back to the beginning so that the request body can be
	// re-read by HTTPClient.Do.
	input.Seek(0, io.SeekStart)
	req.Header.Add("Content-Hash", hash)

	// content-type
	if ct := art.GetContentType(); ct != "" {
		req.Header.Add("Content-Type", ct)
	}

	req.Header.Add("Update-Token", token)
	return req, nil
}

// Upload uploads the the artifact attached to the HTTP request to ResultDB.
func (u *ArtifactUploader) Upload(req *http.Request, art *sinkpb.Artifact) error {
	var resp *http.Response
	resp, err := u.Client.Do(req)
	if err != nil {
		return err
	}
	code := resp.StatusCode
	if code < 400 {
		return nil
	}

	// Tag the error as an transient error, if retriable.
	hErr := errors.Reason("http request failed: %s", resp.Status)
	if code == http.StatusRequestTimeout || code == http.StatusTooManyRequests {
		hErr = hErr.Tag(transient.Tag)
	}
	return hErr.Err()
}

// ReadSeekCloser is an interface that groups the basic Read, Seek, and Close methods.
type ReadSeekCloser interface {
	io.ReadCloser
	io.Seeker
}

// nopCloser is similar to ioutil.nopCloser, but wraps the methods of io.ReadSeeker, instead of io.Reader.
type nopCloser struct {
	io.ReadSeeker
}

func (nopCloser) Close() error { return nil }

// openArtifact returns a readable, seekable, and closerable stream for the artifact.
func openArtifact(art *sinkpb.Artifact) (ReadSeekCloser, int64, error) {
	path := art.GetFilePath()

	if path != "" {
		st, err := os.Stat(path)
		if err != nil {
			return nil, -1, err
		}
		// if the file is small enough to be loaded in memory, load it with
		// bytes.NewBuffer() to avoid unnecessary file operations.
		if st.Size() <= 64*1024 {
			content, err := ioutil.ReadFile(path)
			if err != nil {
				return nil, -1, err
			}
			return nopCloser{bytes.NewReader(content)}, st.Size(), nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, -1, err
		}
		return f, st.Size(), nil
	}
	return nopCloser{bytes.NewReader(art.GetContents())}, int64(len(art.GetContents())), nil
}

func calculateHash(src io.Reader) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, src); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}
