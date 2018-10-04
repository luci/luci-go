// Copyright 2018 The LUCI Authors.
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

package gs

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/net/context/ctxhttp"
	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
)

// TODO(vadimsh): Use this code from the client too.

// RestartUploadError is returned by Uploader when it resumes an interrupted
// upload, and Google Storage asks to upload from an offset the Uploader has no
// data for.
//
// Callers of Uploader should handle this case themselves by restarting the
// upload from the requested offset.
//
// See https://cloud.google.com/storage/docs/json_api/v1/how-tos/resumable-upload#resume-upload
type RestartUploadError struct {
	Offset int64
}

// Error is part of error interface.
func (e *RestartUploadError) Error() string {
	return fmt.Sprintf("the upload should be restarted from offset %d", e.Offset)
}

// Uploader implements io.Writer for Google Storage Resumable Upload sessions.
//
// Does no buffering inside, thus efficiency of uploads directly depends on
// granularity of Write(...) calls. Additionally, Google Storage expects the
// length of each uploaded chunk to be a multiple of 256 Kb, so callers of
// Write(...) should supply the appropriately-sized chunks.
//
// Retries transient errors internally, but it can potentially end up in a
// situation where it needs data not available in the current Write(...)
// operation. In this case Write returns *RestartUploadError error, which
// indicates an offset the upload should be restarted from.
type Uploader struct {
	Context   context.Context // the context for canceling retries and for logging
	Client    *http.Client    // the client to use for sending anonymous requests
	UploadURL string          // upload URL returned by GoogleStorage.StartUpload
	Offset    int64           // offset in the file to upload to, mutated by Write
	FileSize  int64           // total size of the file being uploaded, required

	// requestMock used from tests to mock ctxhttp.Do that is hostile to mocked
	// time.
	requestMock func(*http.Request) (*http.Response, error)
}

// Write is part of io.Writer interface.
func (u *Uploader) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	bufStart := u.Offset
	bufEnd := u.Offset + int64(len(p))
	if bufEnd > u.FileSize {
		return 0, fmt.Errorf("attempting to write past the declared file size (%d > %d)", bufEnd, u.FileSize)
	}

	for u.Offset != bufEnd && err == nil {
		resuming := false
		err = withRetry(u.Context, func() error {
			// When resuming, we upload 0 bytes chunk to grab the last known offset.
			// Otherwise, just upload the next chunk of data.
			var chunk []byte
			if !resuming {
				chunk = p[int(u.Offset-bufStart):]
			}
			resumeOffset, err := u.uploadChunk(chunk)

			// On transient errors, try to resume right away once.
			if apiErr, _ := err.(*googleapi.Error); apiErr != nil && apiErr.Code >= 500 {
				logging.WithError(err).Warningf(u.Context, "Transient error, querying for last uploaded offset")
				resuming = true
				resumeOffset, err = u.uploadChunk(nil)
			}

			switch {
			case err != nil:
				// Either a fatal error during the upload or a transient or fatal error
				// trying to resume. Let 'withRetry' handle it by retrying or failing.
				return err
			case resumeOffset < bufStart || resumeOffset > bufEnd:
				// Resuming requires data we don't have? Escalate to the caller.
				return &RestartUploadError{Offset: resumeOffset}
			default:
				// Resume the upload from the last acknowledged offset.
				u.Offset = resumeOffset
				resuming = false
				return nil
			}
		})
	}

	return int(u.Offset - bufStart), err
}

// uploadChunk pushes the given chunk to Google Storage at u.Offset offset.
//
// Returns an offset to continue the upload from (usually u.Offset + len(p), but
// Google Storage docs are vague about that, so it may be different).
//
// If len(p) is 0, makes an empty PUT request. This is useful for querying
// for the last uploaded offset to resume upload from.
func (u *Uploader) uploadChunk(p []byte) (int64, error) {
	ctx, cancel := clock.WithTimeout(u.Context, 30*time.Second)
	defer cancel()

	logging.Infof(ctx, "gs: UploadChunk(offset=%d, chunk_size=%d, length=%d)", u.Offset, len(p), u.FileSize)
	req, err := http.NewRequest("PUT", u.UploadURL, bytes.NewBuffer(p))
	if err != nil {
		return 0, err
	}
	req.ContentLength = int64(len(p))
	if len(p) > 0 {
		req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", u.Offset, u.Offset+int64(len(p))-1, u.FileSize))
	} else {
		req.Header.Set("Content-Range", fmt.Sprintf("bytes */%d", u.FileSize))
	}

	var resp *http.Response
	if u.requestMock != nil {
		resp, err = u.requestMock(req)
	} else {
		resp, err = ctxhttp.Do(ctx, u.Client, req)
	}
	if err != nil {
		return 0, err
	}
	defer googleapi.CloseBody(resp)

	// Google Storage return 308 (http.StatusPermanentRedirect) on partial upload.
	// Since it is not really a redirect, we just use 308 below to avoid
	// confusion.
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode <= 299:
		return u.FileSize, nil // finished uploading everything
	case resp.StatusCode != 308:
		// Note: we can't call CheckResponse earlier, since it treats 308 as
		// an error.
		if err := googleapi.CheckResponse(resp); err != nil {
			return 0, err
		}
		panic(fmt.Sprintf("impossible state, status code %d", resp.StatusCode))
	}

	// Extract the last uploaded offset from Range header. No Range header means
	// there are no uploaded data yet (and so we need to restart from 0). Be
	// paranoid and check this happens only when we are really resuming. Any
	// successful data upload MUST have Range response header.
	hdr := resp.Header.Get("Range")
	if hdr == "" {
		if len(p) != 0 {
			return 0, fmt.Errorf("no Range header in Google Storage response")
		}
		return 0, nil
	}

	var offset int64
	if _, err = fmt.Sscanf(hdr, "bytes=0-%d", &offset); err != nil {
		return 0, fmt.Errorf("unexpected Range header value: %q", hdr)
	}

	// 'offset' is an offset of the last uploaded byte, need to resume uploading
	// from the next one.
	return offset + 1, nil
}
