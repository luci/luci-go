// Copyright 2017 The LUCI Authors.
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
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/net/context/ctxhttp"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/storage/v1"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"
)

// GoogleStorage is a wrapper over raw Google Cloud Storage JSON API.
//
// Use Get() to grab an implementation.
//
// Uses service's own service account for authentication.
//
// All paths are expected to be in format "/<bucket>/<object>", methods would
// panic otherwise. Use ValidatePath prior to calling GoogleStorage methods
// if necessary.
//
// Errors returned by GoogleStorage are annotated with transient tag (when
// appropriate) and with HTTP status codes of corresponding Google Storage API
// replies (if available). Use StatusCode(err) to extract them.
//
// Retries on transient errors internally a bunch of times. Logs all calls to
// the info log.
type GoogleStorage interface {
	// Exists checks whether given Google Storage file exists.
	Exists(ctx context.Context, path string) (exists bool, err error)

	// Size returns the size in bytes of the given Google Storage file.
	Size(ctx context.Context, path string) (size uint64, exists bool, err error)

	// Copy copies a file at 'src' to 'dst'.
	//
	// Applies ifSourceGenerationMatch and ifGenerationMatch preconditions if
	// srcGen or dstGen are non-negative. See Google Storage docs:
	// https://cloud.google.com/storage/docs/json_api/v1/objects/copy
	Copy(ctx context.Context, dst string, dstGen int64, src string, srcGen int64) error

	// Delete removes a file.
	//
	// Missing file is not an error.
	Delete(ctx context.Context, path string) error

	// Publish implements conditional copy operation with some caveats, making it
	// useful for moving uploaded files from a temporary storage area after they
	// have been verified, ensuring they are not modified during this process.
	//
	// 'src' will be copied to 'dst' only if source generation number matches
	// 'srcGen'. If 'srcGen' is negative, the generation check is not performed.
	// Also assumes 'dst' is ether missing, or already contains data matching
	// 'src' (so 'dst' should usually be a content-addressed path). This allows
	// the conditional move operation to be safely retried even if it failed
	// midway before.
	//
	// Note that it keeps 'src' intact. Use Delete to get rid of it when
	// necessary. Google Storage doesn't have atomic "move" operation.
	Publish(ctx context.Context, dst, src string, srcGen int64) error

	// StartUpload opens a new resumable upload session to a given path.
	//
	// Returns an URL to use in Resumable Upload protocol. It contains uploadId
	// that acts as an authentication token, treat the URL as a secret.
	//
	// The upload protocol is finished by the CIPD client, and so it's not
	// implemented here.
	StartUpload(ctx context.Context, path string) (uploadURL string, err error)

	// CancelUpload cancels a resumable upload session.
	CancelUpload(ctx context.Context, uploadURL string) error

	// Reader returns an io.ReaderAt implementation to read contents of a file at
	// a specific generation (if 'gen' is positive) or at the current live
	// generation (if 'gen' is zero or negative).
	//
	// crbug.com/1261988 - `minSpeed` must be given in bytes-per-second. If > 0,
	// the reader implementation may set an internal timeout to reconnect to GCS
	// in the event that a particular object download RPC is unexpectedly slow. As
	// of 2025Q1, good speeds from GAE to GCS are in the 50MB/s range, but we've
	// seen slow transfer speeds as low as 0.05MB/s, which `minSpeed` is intended
	// to guard against.
	Reader(ctx context.Context, path string, gen, minSpeed int64) (Reader, error)
}

// Reader can read chunks of a Google Storage file.
//
// Use GoogleStorage.Reader to get the reader.
type Reader interface {
	io.ReaderAt

	// Size is the total file size.
	Size() int64
	// Generation is generation number of the content we are reading.
	Generation() int64
}

// impl is actual implementation of GoogleStorage using real API.
type impl struct {
	ctx context.Context

	testingTransport http.RoundTripper // used in tests to mock the transport
	testingBasePath  string            // used in tests to mock Google Storage URL

	o      sync.Once
	err    error            // an initialization error, if any
	client *http.Client     // authenticating HTTP client
	srv    *storage.Service // the raw Cloud Storage API service
}

// Get returns Google Storage JSON API wrapper.
//
// Its guts are lazily initializes on first use, to simplify error handling.
//
// The returned object is associated with the given context and it should not
// outlive it. Each individual method still accepts a context though, which
// can be a derivative of the root context (for example to provide custom
// per-method deadline or logging fields).
func Get(ctx context.Context) GoogleStorage {
	return &impl{ctx: ctx}
}

func (gs *impl) init() error {
	gs.o.Do(func() {
		var err error

		tr := gs.testingTransport
		if tr == nil {
			tr, err = auth.GetRPCTransport(gs.ctx, auth.AsSelf, auth.WithScopes(storage.CloudPlatformScope))
			if err != nil {
				gs.err = errors.Annotate(err, "failed to get authenticating transport").
					Tag(transient.Tag).Err()
				return
			}
		}

		gs.client = &http.Client{Transport: tr}
		gs.srv, err = storage.New(gs.client)
		if err != nil {
			gs.err = errors.Annotate(err, "failed to construct storage.Service").Err()
		} else if gs.testingBasePath != "" {
			gs.srv.BasePath = gs.testingBasePath
		}
	})

	return gs.err
}

func (gs *impl) Size(ctx context.Context, path string) (size uint64, exists bool, err error) {
	logging.Infof(ctx, "gs: Size(path=%q)", path)
	if err := gs.init(); err != nil {
		return 0, false, err
	}

	var obj *storage.Object
	call := gs.srv.Objects.Get(SplitPath(path)).Context(ctx)
	switch err := withRetry(ctx, func() error { obj, err = call.Do(); return err }); {
	case err == nil:
		return obj.Size, true, nil
	case StatusCode(err) == http.StatusNotFound:
		return 0, false, nil
	default:
		return 0, false, err
	}
}

func (gs *impl) Exists(ctx context.Context, path string) (exists bool, err error) {
	// Fetch and discard object size, we are interested in HTTP 404 reply.
	// There's no faster way to check the object presence.
	_, exists, err = gs.Size(ctx, path)
	return exists, err
}

func (gs *impl) Copy(ctx context.Context, dst string, dstGen int64, src string, srcGen int64) error {
	logging.Infof(ctx, "gs: Copy(dst=%q, dstGen=%d, src=%q, srcGen=%d)", dst, dstGen, src, srcGen)
	if err := gs.init(); err != nil {
		return err
	}

	srcBucket, srcPath := SplitPath(src)
	dstBucket, dstPath := SplitPath(dst)

	call := gs.srv.Objects.Copy(srcBucket, srcPath, dstBucket, dstPath, nil).Context(ctx)
	if srcGen >= 0 {
		call.IfSourceGenerationMatch(srcGen)
	}
	if dstGen >= 0 {
		call.IfGenerationMatch(dstGen)
	}
	return withRetry(ctx, func() error { _, err := call.Do(); return err })
}

func (gs *impl) Delete(ctx context.Context, path string) error {
	logging.Infof(ctx, "gs: Delete(path=%q)", path)
	if err := gs.init(); err != nil {
		return err
	}

	call := gs.srv.Objects.Delete(SplitPath(path)).Context(ctx)
	err := withRetry(ctx, func() error { return call.Do() })
	if err == nil || StatusCode(err) == http.StatusNotFound {
		return nil
	}
	return err
}

func (gs *impl) Publish(ctx context.Context, dst, src string, srcGen int64) error {
	err := gs.Copy(ctx, dst, 0, src, srcGen)
	if err == nil {
		return nil
	}
	code := StatusCode(err)

	// srcGen < 0 means we check only precondition on 'dst', so if the copy
	// failed, we already know why: 'dst' already exists (which means the publish
	// operation is successful overall). Other error conditions handled below.
	if code == http.StatusPreconditionFailed && srcGen < 0 {
		return nil
	}

	// StatusNotFound means 'src' is missing. This can happen if we attempted to
	// publish before, failed midway and retrying now. If so, 'dst' should exist
	// already.
	//
	// StatusPreconditionFailed means either 'src' has changed (its generation no
	// longer matches srcGen generation), or 'dst' already exists (its generation
	// is no longer 0).
	//
	// We want to check for 'dst' presence to figure out what to do next.
	//
	// Any other code is unexpected error.
	if code != http.StatusNotFound && code != http.StatusPreconditionFailed {
		return errors.Annotate(err, "failed to copy the object").Err()
	}

	switch _, exists, dstErr := gs.Size(ctx, dst); {
	case dstErr != nil:
		return errors.Annotate(dstErr, "failed to check the destination object presence").Err()
	case !exists && code == http.StatusNotFound:
		// Both 'src' and 'dst' are missing. It means we are not retrying a failed
		// move (it would have left either 'src' or 'dst' or both present), and
		// 'src' is genuinely missing.
		return errors.Annotate(err, "the source object is missing").Err()
	case !exists && code == http.StatusPreconditionFailed:
		// 'dst' is still missing. It means we failed 'srcGen' precondition.
		return errors.Annotate(err, "the source object has unexpected generation number").Err()
	}

	return nil // 'dst' exists, Publish is considered successful
}

func (gs *impl) StartUpload(ctx context.Context, path string) (uploadURL string, err error) {
	logging.Infof(ctx, "gs: StartUpload(path=%q)", path)
	if err := gs.init(); err != nil {
		return "", err
	}

	// Unfortunately gs.srv.Objects.Insert doesn't allow starting a resumable
	// upload session here, but finish it elsewhere. Also it gives no access to
	// the upload URL and demands an io.Reader with all the data right away. So we
	// have to resort to the rawest API possible :(
	//
	// See:
	//   https://cloud.google.com/storage/docs/json_api/v1/objects/insert
	//   https://cloud.google.com/storage/docs/json_api/v1/how-tos/resumable-upload
	bucket, name := SplitPath(path)
	u := &url.URL{
		Scheme: "https",
		Host:   "www.googleapis.com",
		Path:   "/upload/storage/v1/b/{bucket}/o",
		RawQuery: (url.Values{
			"alt":        {"json"},
			"name":       {name},
			"uploadType": {"resumable"},
		}).Encode(),
	}
	googleapi.Expand(u, map[string]string{"bucket": bucket})

	if gs.testingBasePath != "" {
		testingURL, err := url.Parse(gs.testingBasePath)
		if err != nil {
			panic(err)
		}
		u.Scheme = testingURL.Scheme
		u.Host = testingURL.Host
	}

	err = withRetry(ctx, func() error {
		req, _ := http.NewRequest("POST", u.String(), nil)
		resp, err := ctxhttp.Do(ctx, gs.client, req)
		if err != nil {
			return err
		}
		defer googleapi.CloseBody(resp)
		if err := googleapi.CheckResponse(resp); err != nil {
			return err
		}
		uploadURL = resp.Header.Get("Location")
		return nil
	})

	if err != nil {
		return "", errors.Annotate(err, "failed to open the resumable upload session").Err()
	}

	return uploadURL, nil
}

func (gs *impl) CancelUpload(ctx context.Context, uploadURL string) error {
	logging.Infof(ctx, "gs: CancelUpload(uploadURL=%q)", uploadURL)
	if err := gs.init(); err != nil {
		return err
	}

	err := withRetry(ctx, func() error {
		req, _ := http.NewRequest("DELETE", uploadURL, nil)
		resp, err := ctxhttp.Do(ctx, gs.client, req)
		if err != nil {
			return err
		}
		defer googleapi.CloseBody(resp)
		return googleapi.CheckResponse(resp)
	})

	switch {
	case transient.Tag.In(err):
		return err
	case err == nil:
		return errors.Reason("expecting 499 code, but got 200").Err()
	case StatusCode(err) != 499:
		return errors.Annotate(err, "expecting 499 code, but got %d", StatusCode(err)).Err()
	}
	return nil
}

// Reader returns an io.ReaderAt implementation to read contents of a file at
// a specific generation (if 'gen' is positive) or at the current live
// generation (if 'gen' is zero or negative).
func (gs *impl) Reader(ctx context.Context, path string, gen, minSpeed int64) (Reader, error) {
	logging.Infof(ctx, "gs: Reader(path=%q, gen=%d)", path, gen)
	if err := gs.init(); err != nil {
		return nil, err
	}

	// Fetch the object metadata, including its size and the generation number
	// (which is useful when 'gen' is <= 0).
	call := gs.srv.Objects.Get(SplitPath(path)).Context(ctx)
	if gen > 0 {
		call.Generation(gen)
	}
	var obj *storage.Object
	err := withRetry(ctx, func() (err error) { obj, err = call.Do(); return })
	if err != nil {
		return nil, errors.Annotate(err, "failed to grab the object size and generation").Err()
	}

	// Carry on reading from the resolved generation. That way we are not
	// concerned with concurrent changes that may be happening to the file while
	// we are reading it.
	return &readerImpl{
		ctx:      ctx,
		gs:       gs,
		path:     path,
		size:     int64(obj.Size),
		gen:      obj.Generation,
		minSpeed: minSpeed,
	}, nil
}

////////////////////////////////////////////////////////////////////////////////

// readerImpl implements Reader using real APIs.
type readerImpl struct {
	ctx      context.Context
	gs       *impl
	path     string
	size     int64
	gen      int64
	minSpeed int64

	m     sync.Mutex
	speed float64 // moving average, bytes per sec
}

func (r *readerImpl) Size() int64       { return r.size }
func (r *readerImpl) Generation() int64 { return r.gen }

// We always add a base 2s to the timeout to help prevent flake for small reads.
const timeoutConstantExtra = time.Second * 2

func timeoutForSize(minSpeed, size int64) time.Duration {
	if minSpeed <= 0 {
		return 0
	}

	ret := timeoutConstantExtra

	// Then we add the maximum amount of time that we're willing to transfer size.
	ret += (time.Duration(size) * time.Second) / (time.Duration(minSpeed))

	return ret
}

func (r *readerImpl) ReadAt(p []byte, off int64) (n int, err error) {
	toRead := int64(len(p))
	if off+toRead > r.size {
		toRead = r.size - off
	}
	if toRead == 0 {
		return 0, io.EOF
	}
	attemptTimeout := timeoutForSize(r.minSpeed, toRead)

	logging.Debugf(r.ctx, "gs: ReadAt(path=%q, offset=%d, length=%d, gen=%d) - attemptTimeout=%s",
		r.path, off, toRead, r.gen, attemptTimeout)
	if err := r.gs.init(); err != nil {
		return 0, err
	}

	started := time.Now()

	err = withRetry(r.ctx, func() error {
		n = 0

		ctx := r.ctx
		if attemptTimeout > 0 {
			var cancel func()
			ctx, cancel = context.WithTimeoutCause(
				ctx, attemptTimeout, errors.Reason("gs: ReadAt too slow").Tag(transient.Tag).Err())
			defer cancel()
		}

		// 'Download' is magic. Unlike regular call.Do(), it will append alt=media
		// to the request string, thus asking GS to return the object body instead
		// of its metadata.
		call := r.gs.srv.Objects.Get(SplitPath(r.path)).Context(ctx).Generation(r.gen)
		call.Header().Set("Range", fmt.Sprintf("bytes=%d-%d", off, off+toRead-1))
		resp, err := call.Download()
		if err != nil {
			// if this RPC timed out at the beginning due to our timeout, catch it and
			// return the transient error to allow this to retry.
			if cause := context.Cause(ctx); transient.Tag.In(cause) {
				return cause
			}
			return err
		}
		defer googleapi.CloseBody(resp)

		n, err = io.ReadFull(resp.Body, p[:int(toRead)])
		if err != nil {
			return errors.Annotate(err, "failed to read the response").Tag(transient.Tag).Err()
		}

		return nil
	})

	if err == nil && off+toRead == r.size {
		err = io.EOF
	}

	if err == nil {
		r.trackSpeed(toRead, time.Since(started))
	}

	return
}

func (r *readerImpl) trackSpeed(size int64, dt time.Duration) {
	// Ignore small reads, their speed is dominated by noise.
	if size < 64*1024 {
		return
	}

	v := float64(size) / dt.Seconds()

	// Exponential moving average with some arbitrary chosen factor.
	r.m.Lock()
	if r.speed == 0 {
		r.speed = v
	} else {
		r.speed = 0.07*v + 0.93*r.speed
	}
	speed := r.speed
	r.m.Unlock()

	logging.Debugf(r.ctx, "gs: download speed avg[%.2f MB/sec] last_chunk[%.2f MB/sec for %d MB]",
		speed/1e6, v/1e6, size/(1024*1024))
}
