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
	// StreamHost is the host of a ResultDB service instance, to which artifacts are streamed.
	StreamHost string

	// MaxBatchable is the maximum size of an artifact that can be batched.
	MaxBatchable int64
}

// StreamUpload uploads the artifact in a streaming manner via HTTP..
func (u *artifactUploader) StreamUpload(ctx context.Context, t *uploadTask, updateToken string) error {
	var body io.ReadSeeker
	var err error

	if t.art.GetGcsUri() != "" {
		return errors.Reason("StreamUpload does not support gcsUri upload").Err()
	}

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
		req.Body = io.NopCloser(body)
		req.GetBody = func() (io.ReadCloser, error) {
			if _, err := body.Seek(0, io.SeekStart); err != nil {
				return nil, err
			}
			return io.NopCloser(body), nil
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

	// Update the artifact content type if it is missing
	// Read the first 512 bytes since that is what the library uses to
	// determine the type.
	contents := make([]byte, 512)
	bytesRead, err := body.Read(contents)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if _, err := body.Seek(0, io.SeekStart); err != nil {
		return err
	}
	t.art.ContentType = artifactContentType(t.art.ContentType, contents[:bytesRead])
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

// newBatchCreateArtifactsRequest returns a BatchCreateArtifactsRequest with
// at most 500 items with capping the sum of the artifact sizes by maxSum.
//
// Panics if tasks an item with an artifact larger than maxSum.
func newBatchCreateArtifactsRequest(maxSum int64, tasks []buffer.BatchItem) (*pb.BatchCreateArtifactsRequest, error) {
	l := len(tasks)
	if l > 500 {
		l = 500
	}

	var sum int64
	reqs := make([]*pb.CreateArtifactRequest, 0, l)
	for i := 0; i < l; i++ {
		ut := tasks[i].Item.(*uploadTask)

		// artifactChannel.schedule() should have sent it to streamChannel.
		if ut.size > maxSum {
			return nil, errors.Reason("an artifact is greater than %d", maxSum).Err()
		}
		// if the sum is going to be too big, stop the iteration.
		if sum+ut.size > maxSum {
			break
		}

		r, err := ut.CreateRequest()
		if err != nil {
			return nil, errors.Annotate(err, "CreateRequest").Err()
		}
		reqs = append(reqs, r)
		sum += ut.size
	}
	return &pb.BatchCreateArtifactsRequest{Requests: reqs}, nil
}

func (u *artifactUploader) BatchUpload(ctx context.Context, b *buffer.Batch) error {
	var req *pb.BatchCreateArtifactsRequest
	var err error
	if b.Meta != nil {
		req = b.Meta.(*pb.BatchCreateArtifactsRequest)
		if _, err = u.Recorder.BatchCreateArtifacts(ctx, req); err != nil {
			return err
		}
	}

	// There are the following conditions this loop handles.
	//
	// 1) pb.BatchCreateArtifactsRequest can contain at most 500 artifacts.
	// 2) The sum of the artifact content sizes must be <= u.MaxBatchable.
	// 3) The size of input artifacts varies.
	//
	// It's possible that a buffer.Batch contains 500 of large artifact files, like 1MiB.
	// To avoid loading the contents of all the artifacts unnecessarily, this loop slices
	// b.Data by 500 or u.MaxBatchable, and creates a batch request only for the tasks,
	// handled in the current iteration.
	for len(b.Data) > 0 {
		if req, err = newBatchCreateArtifactsRequest(u.MaxBatchable, b.Data); err != nil {
			return errors.Annotate(err, "newBatchCreateArtifactRequest").Err()
		}

		b.Meta = req
		if _, err := u.Recorder.BatchCreateArtifacts(ctx, req); err != nil {
			return err
		}
		b.Data = b.Data[len(req.Requests):]
	}
	return nil
}
