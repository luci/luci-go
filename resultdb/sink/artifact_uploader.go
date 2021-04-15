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
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// artifactUploader provides functions for uploading artifacts to ResultDB.
type artifactUploader struct {
	// Recorder is a gRPC client used to upload artifacts in batches.
	Recorder pb.RecorderClient

	// StreamClient is an HTTP client used to upload artifacts that are too large
	// to be batched.
	StreamClient *http.Client
	// Host is the host of a ResultDB service instance, to which artifacts are streamed.
	StreamHost string
}

// StreamUpload uploads the artifact in a streaming manner via HTTP..
func (u *artifactUploader) StreamUpload(ctx context.Context, t *uploadTask, updateToken string) error {
	var body io.ReadSeeker
	var err error
	if fp := t.art.GetFilePath(); fp == "" {
		body = bytes.NewReader(t.art.GetContents())
	} else {
		fh, err := os.Open(t.art.GetFilePath())
		if err != nil {
			return err
		}
		defer fh.Close()
		body = fh
	}

	req, err := http.NewRequestWithContext(
		ctx, "PUT", fmt.Sprintf("https://%s/%s", u.StreamHost, t.artName), body)
	if err != nil {
		return errors.Annotate(err, "newHTTPRequest").Err()
	}

	// Client.Do always closes the Body on exit, whether there was an error or not.
	// It also closes the Body on 3xx responses, which requires re-sending the request.
	// If req.GetBody != nil, it calls GetBody to get a new copy of the body, and resend
	// the request on 3xx responses.
	//
	// http.NewRequestWithContext() returns an HTTP request with
	// - custom GetBody and ContentLength set, if the given body is of type buffer.Reader,
	// - nil GetBody and ContentLength unset, if the given body is os.File.
	//
	// Therefore, if the body is os.File, the caller is responsible for setting GetBody
	// and ContentLength, as necessary.
	if fh, ok := body.(*os.File); ok {
		// Prevent the file handler from being closed by Client.Do. The file handler, body,
		// will be closed by the defer function above. With NopCloser(), GetBody() can
		// simply reset the cursor and return the handler w/o reopening the file.
		req.Body = ioutil.NopCloser(body)
		req.GetBody = func() (io.ReadCloser, error) {
			if _, err := body.Seek(0, io.SeekStart); err != nil {
				return nil, err
			}
			return ioutil.NopCloser(body), nil
		}

		st, err := fh.Stat()
		if err != nil {
			return err
		}
		req.ContentLength = st.Size()
	}

	// calculates the hash and rewind the position back to the beginning so that
	// the request body can be re-read by HTTPClient.Do.
	hash, err := calculateHash(body)
	if err != nil {
		return errors.Annotate(err, "artifact-hash").Err()
	}
	if _, err := body.Seek(0, io.SeekStart); err != nil {
		return err
	}
	req.Header.Add("Content-Hash", hash)
	if t.art.ContentType != "" {
		req.Header.Add("Content-Type", t.art.ContentType)
	}
	req.Header.Add("Update-Token", updateToken)
	return u.sendHTTP(req)
}

func (u *artifactUploader) sendHTTP(req *http.Request) error {
	return retry.Retry(req.Context(), transient.Only(retry.Default), func() error {
		resp, err := u.StreamClient.Do(req)
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

func (u *artifactUploader) BatchUpload(ctx context.Context, b *buffer.Batch) error {
	// TODO(ddoman): implement me
	return nil
}
