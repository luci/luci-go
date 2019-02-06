// Copyright 2015 The LUCI Authors.
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

package isolatedclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/iotest"

	isolateservice "go.chromium.org/luci/common/api/isolate/isolateservice/v1"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient/isolatedfake"
	"go.chromium.org/luci/common/retry"

	. "github.com/smartystreets/goconvey/convey"
)

// TODO(mknyszek): Add tests for handling http 429.

func TestIsolateServerCaps(t *testing.T) {
	ctx := context.Background()

	t.Parallel()
	Convey(`An empty archiver should produce sane output.`, t, func() {
		server := isolatedfake.New()
		ts := httptest.NewServer(server)
		defer ts.Close()
		client := New(nil, nil, ts.URL, DefaultNamespace, nil, nil)
		caps, err := client.ServerCapabilities(ctx)
		So(err, ShouldBeNil)
		So(caps, ShouldResemble, &isolateservice.HandlersEndpointsV1ServerDetails{ServerVersion: "v1"})
		So(server.Error(), ShouldBeNil)
	})
}

func TestIsolateServerDownloadSmall(t *testing.T) {
	t.Parallel()
	Convey(``, t, func() {
		testNormalDownload(context.Background(), t, foo, bar)
	})
}

func TestIsolateServerDownloadLarge(t *testing.T) {
	t.Parallel()
	Convey(``, t, func() {
		testNormalDownload(context.Background(), t, large)
	})
}

func TestIsolateServerUploadSmall(t *testing.T) {
	t.Parallel()
	Convey(``, t, func() {
		testNormalUpload(context.Background(), t, foo, bar)
	})
}

func TestIsolateServerUploadLarge(t *testing.T) {
	t.Parallel()
	Convey(``, t, func() {
		testNormalUpload(context.Background(), t, large)
	})
}

func TestIsolateServerDownloadRetryGCSPartial(t *testing.T) {
	// GCS download is teared down in the middle.
	t.Parallel()
	Convey(``, t, func() {
		for _, namespace := range []string{"default", "default-gzip", "sha256-foo", "sha512-foo"} {
			Convey(namespace, func() {
				server := isolatedfake.New()
				largeHash := server.Inject(namespace, large)
				flaky := &killingMux{server: server, tearDown: map[string]int{"/fake/cloudstorage/download": 1024}}
				flaky.ts = httptest.NewServer(flaky)
				defer flaky.ts.Close()
				client := New(nil, nil, flaky.ts.URL, namespace, fastRetry, nil)

				var buffer bytes.Buffer
				So(client.Fetch(context.Background(), largeHash, &buffer), ShouldBeNil)
				So(server.Error(), ShouldBeNil)
				So(buffer.Bytes(), ShouldResemble, large)
				So(flaky.tearDown, ShouldResemble, map[string]int{})
				So(client.Hash(), ShouldEqual, isolated.GetHash(namespace))
			})
		}
	})
}

func TestIsolateServerUploadRetryGCSPartial(t *testing.T) {
	ctx := context.Background()

	// GCS upload is teared down in the middle.
	t.Parallel()
	Convey(``, t, func() {
		server := isolatedfake.New()
		namespace := DefaultNamespace
		flaky := &killingMux{server: server, tearDown: map[string]int{"/fake/cloudstorage/upload": 1024}}
		flaky.ts = httptest.NewServer(flaky)
		defer flaky.ts.Close()
		client := New(nil, nil, flaky.ts.URL, namespace, fastRetry, nil)

		digests, contents, expected := makeItems(namespace, large)
		states, err := client.Contains(ctx, digests)
		So(err, ShouldBeNil)
		So(len(states), ShouldResemble, len(digests))
		for _, state := range states {
			err = client.Push(ctx, state, NewBytesSource(contents[state.status.Index]))
			So(err, ShouldBeNil)
		}
		So(server.Contents(), ShouldResemble, expected)
		So(flaky.tearDown, ShouldResemble, map[string]int{})

		// Look up again to confirm.
		states, err = client.Contains(ctx, digests)
		So(err, ShouldBeNil)
		So(len(states), ShouldResemble, len(digests))
		for _, state := range states {
			So(state, ShouldResemble, (*PushState)(nil))
		}
		So(server.Error(), ShouldBeNil)
	})
}

func TestIsolateServerBadURL(t *testing.T) {
	t.Parallel()
	Convey(``, t, func() {
		if testing.Short() {
			t.SkipNow()
		}
		client := New(nil, nil, "http://127.0.0.1:1", DefaultNamespace, fastRetry, nil)
		caps, err := client.ServerCapabilities(context.Background())
		So(caps, ShouldBeNil)
		So(err, ShouldNotBeNil)
	})
}

func TestRetryDownload(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	type testCase struct {
		name         string
		item         []byte // item to Inject into the server, and which will be downloaded.
		errURL       string // a request to this URL is expected, and we will serve a 503 once before succeeding on retries.
		fatalURL     string // the test will fail if this URL is requested.
		retryFactory func() retry.Iterator
		failure      bool // expected result of calling testFunc.
	}

	// testFunc injects and then attempts to download tc.item off the server.
	// Failure may be induced at various points in this process by setting tc.errURL and tc.retryFactory.
	// Requests to tc.errURL will return a 503 once before succeeding.
	// tc.retryFactory will be used to decide whether to retry these failed requests.
	// testFunc will return a failStep which indicates which step in the process failed.
	testFunc := func(tc testCase) bool {
		// Construct a server which will serve a single 503 for requests to tc.errURL before succeeding.
		server := isolatedfake.New()
		namespace := DefaultNamespace
		itemHash := server.Inject(namespace, tc.item)
		flaky := &killingMux{server: server, http503: make(map[string]int)}
		if tc.errURL != "" {
			flaky.http503[tc.errURL] = 10
		}
		flaky.ts = httptest.NewServer(flaky)
		defer flaky.ts.Close()
		client := New(nil, nil, flaky.ts.URL, namespace, tc.retryFactory, nil)

		var buffer bytes.Buffer
		err := client.Fetch(ctx, itemHash, &buffer)
		So(server.Error(), ShouldBeNil)
		if err != nil {
			return true
		}
		So(buffer.Bytes(), ShouldResemble, tc.item)

		_, fatalURLSeen := flaky.seenURLs[tc.fatalURL]
		So(fatalURLSeen, ShouldBeFalse)
		So(flaky.http503, ShouldResemble, map[string]int{})

		return false
	}

	Convey(``, t, func() {
		testCases := []testCase{
			{
				name:         "retry retrieve",
				item:         small,
				errURL:       "/_ah/api/isolateservice/v1/retrieve",
				retryFactory: fastRetry,
				failure:      false,
			},
			{
				name:         "no retry retrieve",
				item:         small,
				errURL:       "/_ah/api/isolateservice/v1/retrieve",
				retryFactory: noRetry,
				failure:      true,
			},
			{
				name:         "large files should give up being retrieved just like small ones",
				item:         large,
				errURL:       "/_ah/api/isolateservice/v1/retrieve",
				retryFactory: noRetry,
				failure:      true,
			},
			{
				name:         "gs download failure is irrelevant for small files",
				item:         small,
				fatalURL:     "/fake/cloudstorage/download",
				retryFactory: noRetry,
				failure:      false,
			},
			{
				name:         "gs download failure is relevant for large files",
				item:         large,
				errURL:       "/fake/cloudstorage/download",
				retryFactory: noRetry,
				failure:      true,
			},
			{
				name:         "retry gs download",
				item:         large,
				errURL:       "/fake/cloudstorage/download",
				retryFactory: fastRetry,
				failure:      false,
			},
		}
		for _, tc := range testCases {
			tc := tc
			Convey(tc.name, func() {
				So(testFunc(tc), ShouldEqual, tc.failure)
			})
		}
	})
}

func TestRetryUpload(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// failStep indicates the point at which testFunc failed.
	type failStep int
	const (
		success        failStep = iota
		failedContains failStep = iota
		failedPush     failStep = iota
	)

	type testCase struct {
		name         string
		items        [][]byte // items to upload
		errURL       string   // a request to this URL is expected, and we will serve a 503 once before succeeding on retries.
		fatalURL     string   // the test will fail if this URL is requested.
		retryFactory func() retry.Iterator
		result       failStep // expected result of calling testFunc.
	}

	// testFunc checks for items on the server, and then uploads those that are missing (i.e. all of them).
	// Failure may be induced at various points in this process by setting tc.errURL and tc.retryFactory.
	// Requests to tc.errURL will return a 503 once before succeeding.
	// tc.retryFactory will be used to decide whether to retry these failed requests.
	// testFunc will return a failStep which indicates which step in the process failed.
	testFunc := func(tc testCase) failStep {
		// Construct a server which will serve a single 503 for requests to tc.errURL before succeeding.
		server := isolatedfake.New()
		namespace := DefaultNamespace
		flaky := &killingMux{server: server, http503: make(map[string]int)}
		if tc.errURL != "" {
			flaky.http503[tc.errURL] = 10
		}
		flaky.ts = httptest.NewServer(flaky)
		defer flaky.ts.Close()
		client := New(nil, nil, flaky.ts.URL, namespace, tc.retryFactory, nil)

		digests, contents, expected := makeItems(namespace, tc.items...)
		states, err := client.Contains(ctx, digests)
		if err != nil {
			return failedContains
		}
		So(len(states), ShouldResemble, len(digests))

		for _, state := range states {
			err = client.Push(ctx, state, NewBytesSource(contents[state.status.Index]))
			if err != nil {
				return failedPush
			}
		}
		So(server.Contents(), ShouldResemble, expected)

		states, err = client.Contains(ctx, digests)
		So(err, ShouldBeNil)
		So(len(states), ShouldResemble, len(digests))
		for _, state := range states {
			So(state, ShouldBeNil)
		}
		So(server.Error(), ShouldBeNil)

		_, fatalURLSeen := flaky.seenURLs[tc.fatalURL]
		So(fatalURLSeen, ShouldBeFalse)
		So(flaky.http503, ShouldResemble, map[string]int{})

		return success
	}

	Convey(``, t, func() {
		testCases := []testCase{
			{
				name:         "retry contains",
				items:        [][]byte{small},
				errURL:       "/_ah/api/isolateservice/v1/preupload",
				retryFactory: fastRetry,
				result:       success,
			},
			{
				name:         "skip retry of contains",
				items:        [][]byte{small},
				errURL:       "/_ah/api/isolateservice/v1/preupload",
				retryFactory: noRetry,
				result:       failedContains,
			},

			{
				name:         "retry store_inline",
				items:        [][]byte{small},
				errURL:       "/_ah/api/isolateservice/v1/store_inline",
				retryFactory: fastRetry,
				result:       success,
			},
			{
				name:         "skip retry of store_inline",
				items:        [][]byte{small},
				errURL:       "/_ah/api/isolateservice/v1/store_inline",
				retryFactory: noRetry,
				result:       failedPush,
			},
			{
				name:         "store_inline failure is irrelevant for large files",
				items:        [][]byte{large},
				fatalURL:     "/_ah/api/isolateservice/v1/store_inline",
				retryFactory: noRetry,
				result:       success,
			},

			{
				name:         "retry gs upload",
				items:        [][]byte{large},
				errURL:       "/fake/cloudstorage/upload",
				retryFactory: fastRetry,
				result:       success,
			},
			{
				name:         "skip retry of gs upload",
				items:        [][]byte{large},
				errURL:       "/fake/cloudstorage/upload",
				retryFactory: noRetry,
				result:       failedPush,
			},
			{
				name:         "gs upload failure is irrelevant for small files",
				items:        [][]byte{small},
				fatalURL:     "/fake/cloudstorage/upload",
				retryFactory: noRetry,
				result:       success,
			},

			{
				name:         "retry finalize",
				items:        [][]byte{large}, // Must use large, because finalize is not called for small objects.
				errURL:       "/_ah/api/isolateservice/v1/finalize_gs_upload",
				retryFactory: fastRetry,
				result:       success,
			},
			{
				name:         "skip retry of finalize",
				items:        [][]byte{large},
				errURL:       "/_ah/api/isolateservice/v1/finalize_gs_upload",
				retryFactory: noRetry,
				result:       failedPush,
			},
		}
		for _, tc := range testCases {
			tc := tc
			Convey(tc.name, func() {
				So(testFunc(tc), ShouldEqual, tc.result)
			})
		}
	})
}

// Private stuff.

func noRetry() retry.Iterator {
	return &retry.Limited{
		Retries: 0,
	}
}

func fastRetry() retry.Iterator {
	return &retry.Limited{
		Retries: 10,
	}
}

var (
	bar   = []byte("bar")
	foo   = []byte("foo")
	small = []byte("small")
	large []byte
)

func init() {
	src := rand.New(rand.NewSource(0))
	// It has to be exactly 64kb + 1; as the internal buffering is 64kb. This is
	// needed to reproduce https://crbug.com/552697
	large = make([]byte, 64*1024+1)
	for i := 0; i < len(large); i++ {
		large[i] = byte(src.Uint32())
	}
}

func makeItems(namespace string, contents ...[]byte) ([]*isolateservice.HandlersEndpointsV1Digest, [][]byte, map[string]map[isolated.HexDigest][]byte) {
	digests := make([]*isolateservice.HandlersEndpointsV1Digest, 0, len(contents))
	expected := map[string]map[isolated.HexDigest][]byte{
		namespace: make(map[isolated.HexDigest][]byte, len(contents)),
	}
	h := isolated.GetHash(namespace)
	for _, content := range contents {
		hex := isolated.HashBytes(h, content)
		digests = append(digests, &isolateservice.HandlersEndpointsV1Digest{Digest: string(hex), IsIsolated: false, Size: int64(len(content))})
		expected[namespace][hex] = content
	}
	return digests, contents, expected
}

func testNormalDownload(ctx context.Context, t *testing.T, contents ...[]byte) {
	Convey(``, func() {
		server := isolatedfake.New()
		namespace := DefaultNamespace
		hashes := make([]isolated.HexDigest, 0, len(contents))
		for _, file := range contents {
			hashes = append(hashes, server.Inject(namespace, file))
		}
		ts := httptest.NewServer(server)
		defer ts.Close()
		client := New(nil, nil, ts.URL, namespace, noRetry, nil)
		for i, hash := range hashes {
			var buffer bytes.Buffer
			err := client.Fetch(ctx, hash, &buffer)
			So(err, ShouldBeNil)
			So(buffer.Bytes(), ShouldResemble, contents[i])
		}
		So(server.Error(), ShouldBeNil)
	})
}

func testNormalUpload(ctx context.Context, t *testing.T, contents ...[]byte) {
	Convey(``, func() {
		namespace := DefaultNamespace
		digests, _, expected := makeItems(namespace, contents...)
		server := isolatedfake.New()
		ts := httptest.NewServer(server)
		defer ts.Close()
		client := New(nil, nil, ts.URL, namespace, noRetry, nil)
		states, err := client.Contains(ctx, digests)
		So(err, ShouldBeNil)
		So(len(states), ShouldResemble, len(digests))
		for _, state := range states {
			// The data is automatically compressed.
			err = client.Push(ctx, state, NewBytesSource(contents[state.status.Index]))
			So(err, ShouldBeNil)
		}
		So(server.Error(), ShouldBeNil)
		So(server.Contents(), ShouldResemble, expected)
		states, err = client.Contains(ctx, digests)
		So(err, ShouldBeNil)
		So(len(states), ShouldResemble, len(digests))
		for _, state := range states {
			So(state, ShouldBeNil)
		}
		So(server.Error(), ShouldBeNil)
	})
}

// killingMux inserts tears down connection in the middle of a transfer.
type killingMux struct {
	lock     sync.Mutex
	server   http.Handler
	http503  map[string]int // Number of bytes should be read before returning HTTP 500.
	tearDown map[string]int // Number of bytes should be read before killing the connection.
	seenURLs map[string]struct{}
	ts       *httptest.Server
}

func readBytes(r io.Reader, toRead int) {
	b := make([]byte, toRead)
	read := 0
	for read < toRead {
		n, _ := r.Read(b[:toRead-read])
		if n == 0 {
			return
		}
		read += n
	}
}

func (k *killingMux) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	f := func() bool {
		k.lock.Lock()
		defer k.lock.Unlock()
		if k.seenURLs == nil {
			k.seenURLs = make(map[string]struct{})
		}
		k.seenURLs[req.URL.Path] = struct{}{}

		if toRead, ok := k.http503[req.URL.Path]; ok {
			delete(k.http503, req.URL.Path)
			readBytes(req.Body, toRead)
			w.WriteHeader(503)
			return false
		}
		if toRead, ok := k.tearDown[req.URL.Path]; ok {
			delete(k.tearDown, req.URL.Path)
			readBytes(req.Body, toRead)
			k.ts.CloseClientConnections()
			return false
		}
		return true
	}
	if f() {
		log.Printf("%-4s %s", req.Method, req.URL.Path)
		k.server.ServeHTTP(w, req)
	}
}

type testReadCloser struct {
	io.Reader
	CloseErr error // The error to return on close, if any.
	Closed   bool  // True if close has been called.
}

func (t *testReadCloser) Close() error {
	t.Closed = true
	return t.CloseErr
}

func TestCompressor(t *testing.T) {
	errBang := errors.New("bang")

	testCases := []struct {
		Desc         string
		Src          io.Reader // The Closer is added by the test.
		ReadN        int64     // Bytes to read (negative to read all, zero to read none).
		WantReadErr  error     // The expected error from reading.
		CloseErr     error     // The error to return from close.
		WantCloseErr error     // The expected error from close.
	}{
		{
			Desc:  "all contents read",
			Src:   strings.NewReader("I am a simple sample reader"),
			ReadN: -1,
		},
		{
			Desc:  "no contents read",
			Src:   strings.NewReader("I am a simple sample reader"),
			ReadN: 0,
		},
		{
			Desc:  "partial contents read",
			Src:   strings.NewReader("I am a simple sample reader"),
			ReadN: 10,
		},
		{
			Desc:         "error on source close",
			Src:          strings.NewReader("I am a simple sample reader"),
			ReadN:        -1,
			CloseErr:     errBang,
			WantCloseErr: errBang,
		},
		{
			Desc:        "error on read",
			Src:         iotest.TimeoutReader(iotest.OneByteReader(strings.NewReader("asd"))),
			ReadN:       -1,
			WantReadErr: iotest.ErrTimeout,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Desc, func(t *testing.T) {
			src := &testReadCloser{
				Reader:   tc.Src,
				CloseErr: tc.CloseErr,
			}
			namespace := DefaultNamespace
			comp := newCompressed(namespace, src)

			var readErr error
			switch {
			case tc.ReadN < 0:
				_, readErr = ioutil.ReadAll(comp)
			case tc.ReadN > 0:
				_, readErr = io.CopyN(ioutil.Discard, comp, tc.ReadN)
			}
			if readErr != tc.WantReadErr {
				t.Fatalf("Read error: got: %v; want: %v", readErr, tc.WantReadErr)
			}

			if err := comp.Close(); err != tc.WantCloseErr {
				t.Fatalf("Close error: got: %v; want: %v", err, tc.WantCloseErr)
			}
			if !src.Closed {
				t.Errorf("Underlying source was not closed")
			}
		})
	}
}
