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

	// MaxBatchable is the maximum size of an artifact that can be batched.
	MaxBatchable int64
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

// uploadTaskSlicer slices upload tasks and creates BatchCreateArtifactsRequest for
// each chunk.
//
// Each chunk can have at most 500 artifacts and the total size is capped
// at MaxBatchableArtifactSize.
type uploadTaskSlicer struct {
	batchReq *pb.BatchCreateArtifactsRequest
	pos      int
	tasks    []interface{}

	// maxBatchable is the maximum size of an artifact that can be batched.
	// See artifactChannel.maxBatchable for more info.
	maxBatchable int64
}

// BatchCreateRequest returns a BatchCreateArtifactsRequest for the current slice of the tasks.
//
// Use Advance to move the cursor to the next chunk.
// Returns nil if there are no more tasks to be sliced.
func (s *uploadTaskSlicer) BatchCreateRequest() (*pb.BatchCreateArtifactsRequest, error) {
	if s.batchReq != nil {
		return s.batchReq, nil
	}
	l := len(s.tasks) - s.pos
	switch {
	case l > 500:
		l = 500
	case l == 0:
		// Nothing to batch.
		return nil, nil
	}
	reqs := make([]*pb.CreateArtifactRequest, 0, l)
	var sizeTotal int64
	for _, t := range s.tasks[s.pos:] {
		ut := t.(*uploadTask)
		r, err := ut.CreateRequest()
		if err != nil {
			return nil, errors.Annotate(err, "BatchCreateRequest").Err()
		}

		if ut.size > s.maxBatchable {
			// artifactChannel.schedule() should have sent it to streamChannel.
			panic(fmt.Sprintf("An artifact cannot be bigger than %d", s.maxBatchable))
		}

		if sizeTotal+ut.size > s.maxBatchable || len(reqs) > 500 {
			break
		}
		reqs = append(reqs, r)
		sizeTotal += ut.size
	}
	s.pos += len(reqs)
	s.batchReq = &pb.BatchCreateArtifactsRequest{Requests: reqs}
	return s.batchReq, nil
}

// Advance moves the cursor to point the next slice in the upload tasks.
func (s *uploadTaskSlicer) Advance() {
	s.batchReq = nil
}

func (u *artifactUploader) BatchUpload(ctx context.Context, b *buffer.Batch) error {
	if b.Meta == nil {
		b.Meta = &uploadTaskSlicer{tasks: b.Data, maxBatchable: u.MaxBatchable}
	}

	s := b.Meta.(*uploadTaskSlicer)
	for r, e := s.BatchCreateRequest(); r != nil; r, e = s.BatchCreateRequest() {
		if e != nil {
			return e
		}
		if _, e = u.Recorder.BatchCreateArtifacts(ctx, r); e != nil {
			return e
		}
		s.Advance()
	}
	return nil
}
