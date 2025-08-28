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
	"net/url"
	"os"
	"strings"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"

	"go.chromium.org/luci/resultdb/pbutil"
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
		return errors.New("StreamUpload does not support gcsUri upload")
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
		ctx, "POST", fmt.Sprintf("https://%s/%s/artifacts", u.StreamHost, pbutil.InvocationName(t.invocationID)), body)
	if err != nil {
		return errors.Fmt("newHTTPRequest: %w", err)
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

	if t.testID != nil {
		req.Header.Add("Test-Module-Name", url.PathEscape(t.testID.ModuleName))
		req.Header.Add("Test-Module-Scheme", url.PathEscape(t.testID.ModuleScheme))

		// Variant is transferred as a comma-separated list of URL-encoded string pairs.
		if len(t.testID.ModuleVariant.Def) > 0 {
			var variantHeaderValue strings.Builder
			for k, v := range t.testID.ModuleVariant.Def {
				stringPair := url.PathEscape(fmt.Sprintf("%s:%s", k, v))
				if variantHeaderValue.Len() > 0 {
					variantHeaderValue.WriteString(",")
				}
				variantHeaderValue.WriteString(stringPair)
			}
			req.Header.Add("Test-Module-Variant", variantHeaderValue.String())
		}
		req.Header.Add("Test-Coarse-Name", url.PathEscape(t.testID.CoarseName))
		req.Header.Add("Test-Fine-Name", url.PathEscape(t.testID.FineName))
		req.Header.Add("Test-Case-Name", url.PathEscape(t.testID.CaseName))
		req.Header.Add("Result-ID", t.resultID)
	}
	req.Header.Add("Artifact-ID", t.artifactID)

	// calculates the hash and rewind the position back to the beginning so that
	// the request body can be re-read by HTTPClient.Do.
	hash, err := calculateHash(body)
	if err != nil {
		return errors.Fmt("artifact-hash: %w", err)
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
			return errors.Fmt("failed to send HTTP request: %w", err)
		}

		code := resp.StatusCode
		// ResultDB returns StatusCreated on success.
		if code == http.StatusCreated {
			return nil
		}

		// Try to read the detailed error message.
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			// Ignore error, we already have another error.
		}
		detailedError := strings.TrimSpace(string(body))

		// Tag the error as an transient error, if retriable.
		err = errors.Fmt("http request failed(%d): %s", resp.StatusCode, detailedError)
		if code == http.StatusRequestTimeout || code == http.StatusTooManyRequests || code >= 500 {
			err = transient.Tag.Apply(err)
		}
		return err
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
// at most 500 items with capping the sum of the request sizes to no more than
// maxSum.
//
// Panics if tasks an item with an artifact larger than maxSum.
func newBatchCreateArtifactsRequest(maxSum int64, tasks []buffer.BatchItem[*uploadTask]) (*pb.BatchCreateArtifactsRequest, error) {
	l := len(tasks)
	if l > 500 {
		l = 500
	}

	if maxSum > pbutil.MaxBatchRequestSize {
		return nil, errors.Fmt("requested maxSum (%d bytes) exceeds maximum ResultDB batch size (%d bytes)", maxSum, pbutil.MaxBatchRequestSize)
	}

	var requestSize int64
	reqs := make([]*pb.CreateArtifactRequest, 0, l)
	for i := 0; i < l; i++ {
		ut := tasks[i].Item

		// artifactChannel.schedule() should have sent it to streamChannel.
		if ut.size > (maxSum - batchedArtifactOverheadBytes) {
			return nil, errors.Fmt("an artifact is greater than %d", maxSum)
		}

		estimatedSize := ut.size + batchedArtifactOverheadBytes

		// We have hit our soft request size limit, end the batch.
		if requestSize+estimatedSize > maxSum {
			break
		}

		r, err := ut.CreateRequest()
		if err != nil {
			return nil, errors.Fmt("CreateRequest: %w", err)
		}

		reqs = append(reqs, r)
		requestSize += int64(proto.Size(r))
	}
	return &pb.BatchCreateArtifactsRequest{Requests: reqs}, nil
}

func (u *artifactUploader) BatchUpload(ctx context.Context, b *buffer.Batch[*uploadTask]) error {
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
			return errors.Fmt("newBatchCreateArtifactRequest: %w", err)
		}

		b.Meta = req
		if _, err := u.Recorder.BatchCreateArtifacts(ctx, req); err != nil {
			return err
		}
		b.Data = b.Data[len(req.Requests):]
	}
	return nil
}
