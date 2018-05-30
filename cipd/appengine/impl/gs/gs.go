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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"

	"golang.org/x/net/context"
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
	Exists(c context.Context, path string) (exists bool, err error)

	// Copy copies a file at 'src' to 'dst'.
	//
	// Applies ifSourceGenerationMatch and ifGenerationMatch preconditions if
	// srcGen or dstGen are non-negative. See Google Storage docs:
	// https://cloud.google.com/storage/docs/json_api/v1/objects/copy
	Copy(c context.Context, dst string, dstGen int64, src string, srcGen int64) error

	// Delete removes a file.
	//
	// Missing file is not an error.
	Delete(c context.Context, path string) error

	// Publish implements conditional copy operation with some caveats, making it
	// useful for moving uploaded files from a temporary storage area after they
	// have been verified, ensuring they are not modified during this process.
	//
	// 'src' will be copied to 'dst' only if source generation number matches
	// 'srcGen'. If 'srcGen' is negative, the generation check is not performed.
	// Also assumes 'dst' is ether missing, or already contains data matching 'src'
	// (so 'dst' should usually be a content-addressed path). This allows the
	// conditional move operation to be safely retried even if it failed midway
	// before.
	//
	// Note that it keeps 'src' intact. Use Delete to get rid of it when necessary.
	// Google Storage doesn't have atomic "move" operation.
	Publish(c context.Context, dst, src string, srcGen int64) error

	// StartUpload opens a new resumable upload session to a given path.
	//
	// Returns an URL to use in Resumable Upload protocol. It contains uploadId that
	// acts as an authentication token, treat the URL as a secret.
	//
	// The upload protocol is finished by the CIPD client, and so it's not
	// implemented here.
	StartUpload(c context.Context, path string) (uploadURL string, err error)

	// CancelUpload cancels a resumable upload session.
	CancelUpload(c context.Context, uploadURL string) error

	// Reader returns an io.ReaderAt implementation to read contents of a file at
	// a specific generation (if 'gen' is positive) or at the current live
	// generation (if 'gen' is zero or negative).
	Reader(c context.Context, path string, gen int64) (Reader, error)
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
	c context.Context

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
func Get(c context.Context) GoogleStorage {
	return &impl{c: c}
}

func (gs *impl) init() error {
	gs.o.Do(func() {
		var err error

		tr := gs.testingTransport
		if tr == nil {
			tr, err = auth.GetRPCTransport(gs.c, auth.AsSelf, auth.WithScopes(storage.CloudPlatformScope))
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

func (gs *impl) Exists(c context.Context, path string) (exists bool, err error) {
	logging.Infof(c, "gs: Exists(path=%q)", path)
	if err := gs.init(); err != nil {
		return false, err
	}

	// Fetch and discard object metadata, we are interested in HTTP 404 reply.
	// There's no faster way to check the object presence.
	call := gs.srv.Objects.Get(SplitPath(path)).Context(c)
	switch err := withRetry(c, func() error { _, err = call.Do(); return err }); {
	case err == nil:
		return true, nil
	case StatusCode(err) == http.StatusNotFound:
		return false, nil
	default:
		return false, err
	}
}

func (gs *impl) Copy(c context.Context, dst string, dstGen int64, src string, srcGen int64) error {
	logging.Infof(c, "gs: Copy(dst=%q, dstGen=%d, src=%q, srcGen=%d)", dst, dstGen, src, srcGen)
	if err := gs.init(); err != nil {
		return err
	}

	srcBucket, srcPath := SplitPath(src)
	dstBucket, dstPath := SplitPath(dst)

	call := gs.srv.Objects.Copy(srcBucket, srcPath, dstBucket, dstPath, nil).Context(c)
	if srcGen >= 0 {
		call.IfSourceGenerationMatch(srcGen)
	}
	if dstGen >= 0 {
		call.IfGenerationMatch(dstGen)
	}
	return withRetry(c, func() error { _, err := call.Do(); return err })
}

func (gs *impl) Delete(c context.Context, path string) error {
	logging.Infof(c, "gs: Delete(path=%q)", path)
	if err := gs.init(); err != nil {
		return err
	}

	call := gs.srv.Objects.Delete(SplitPath(path)).Context(c)
	err := withRetry(c, func() error { return call.Do() })
	if err == nil || StatusCode(err) == http.StatusNotFound {
		return nil
	}
	return err
}

func (gs *impl) Publish(c context.Context, dst, src string, srcGen int64) error {
	switch err := gs.Copy(c, dst, -1, src, srcGen); {
	case StatusCode(err) == http.StatusNotFound:
		// 'src' is missing. Maybe we attempted to publish already and this is a
		// retry. If so, 'dst' should exist already, and we assume it matches 'src'
		// per the method contract. Note that there's no efficient way to check
		// this other than fetching and comparing files (which is insanely costly),
		// so we assume callers consistently provide same 'dst' when retrying.
		//
		// If 'dst' is also missing, then we are not retying a failed move (it would
		// have left either 'src' or 'dst' or both present), and 'src' is genuinely
		// missing.
		switch exists, dstErr := gs.Exists(c, dst); {
		case dstErr != nil:
			return errors.Annotate(dstErr, "failed to check the destination object presence").Err()
		case !exists:
			return errors.Annotate(err, "the source object is missing").Err()
		default:
			return nil // 'dst' exists, Publish is considered successful
		}
	case StatusCode(err) == http.StatusPreconditionFailed:
		return errors.Annotate(err, "the source object has unexpected generation number").Err()
	case err != nil:
		return errors.Annotate(err, "failed to copy the object").Err()
	default:
		return nil
	}
}

func (gs *impl) StartUpload(c context.Context, path string) (uploadURL string, err error) {
	logging.Infof(c, "gs: StartUpload(path=%q)", path)
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

	err = withRetry(c, func() error {
		req, _ := http.NewRequest("POST", u.String(), nil)
		resp, err := ctxhttp.Do(c, gs.client, req)
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

func (gs *impl) CancelUpload(c context.Context, uploadURL string) error {
	logging.Infof(c, "gs: CancelUpload(uploadURL=%q)", uploadURL)
	if err := gs.init(); err != nil {
		return err
	}

	err := withRetry(c, func() error {
		req, _ := http.NewRequest("DELETE", uploadURL, nil)
		resp, err := ctxhttp.Do(c, gs.client, req)
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
func (gs *impl) Reader(c context.Context, path string, gen int64) (Reader, error) {
	logging.Infof(c, "gs: Reader(path=%q, gen=%d)", path, gen)
	if err := gs.init(); err != nil {
		return nil, err
	}

	// Fetch the object metadata, including its size and the generation number
	// (which is useful when 'gen' is <= 0).
	call := gs.srv.Objects.Get(SplitPath(path)).Context(c)
	if gen > 0 {
		call.Generation(gen)
	}
	var obj *storage.Object
	err := withRetry(c, func() (err error) { obj, err = call.Do(); return })
	if err != nil {
		return nil, errors.Annotate(err, "failed to grab the object size and generation").Err()
	}

	// Carry on reading from the resolved generation. That way we are not
	// concerned with concurrent changes that may be happening to the file while
	// we are reading it.
	return &readerImpl{c, gs, path, int64(obj.Size), obj.Generation}, nil
}

////////////////////////////////////////////////////////////////////////////////

// readerImpl implements Reader using real APIs.
type readerImpl struct {
	c    context.Context
	gs   *impl
	path string
	size int64
	gen  int64
}

func (r *readerImpl) Size() int64       { return r.size }
func (r *readerImpl) Generation() int64 { return r.gen }

func (r *readerImpl) ReadAt(p []byte, off int64) (n int, err error) {
	toRead := int64(len(p))
	if off+toRead > r.size {
		toRead = r.size - off
	}
	if toRead == 0 {
		return 0, io.EOF
	}

	logging.Debugf(r.c, "gs: ReadAt(path=%q, offset=%d, length=%d, gen=%d)", r.path, off, toRead, r.gen)
	if err := r.gs.init(); err != nil {
		return 0, err
	}

	call := r.gs.srv.Objects.Get(SplitPath(r.path)).Context(r.c).Generation(r.gen)
	call.Header().Set("Range", fmt.Sprintf("bytes=%d-%d", off, off+toRead-1))

	err = withRetry(r.c, func() error {
		n = 0
		// 'Download' is magic. Unlike regular call.Do(), it will append alt=media
		// to the request string, thus asking GS to return the object body instead
		// of its metadata.
		resp, err := call.Download()
		if err != nil {
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
	return
}
