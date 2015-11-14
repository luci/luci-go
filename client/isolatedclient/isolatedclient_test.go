// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolatedclient

import (
	"bytes"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/luci/luci-go/client/internal/retry"
	"github.com/luci/luci-go/client/isolatedclient/isolatedfake"
	"github.com/luci/luci-go/common/isolated"
	"github.com/maruel/ut"
)

func TestIsolateServerCaps(t *testing.T) {
	t.Parallel()
	server := isolatedfake.New()
	ts := httptest.NewServer(server)
	defer ts.Close()
	client := New(ts.URL, "default-gzip")
	caps, err := client.ServerCapabilities()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, &isolated.ServerCapabilities{"v1"}, caps)
	ut.AssertEqual(t, nil, server.Error())
}

func TestIsolateServerSmall(t *testing.T) {
	t.Parallel()
	testNormal(t, foo, bar)
}

func TestIsolateServerLarge(t *testing.T) {
	t.Parallel()
	testNormal(t, large)
}

func TestIsolateServerRetryContains(t *testing.T) {
	t.Parallel()
	testFlaky(t, "/_ah/api/isolateservice/v1/preupload")
}

func TestIsolateServerRetryStoreInline(t *testing.T) {
	t.Parallel()
	testFlaky(t, "/_ah/api/isolateservice/v1/store_inline")
}

func TestIsolateServerRetryGCS(t *testing.T) {
	t.Parallel()
	testFlaky(t, "/fake/cloudstorage")
}

func TestIsolateServerRetryFinalize(t *testing.T) {
	t.Parallel()
	testFlaky(t, "/_ah/api/isolateservice/v1/finalize_gs_upload")
}

func TestIsolateServerRetryGCSPartial(t *testing.T) {
	// GCS upload is teared down in the middle.
	t.Parallel()
	server := isolatedfake.New()
	flaky := &killingMux{server: server, tearDown: map[string]int{"/fake/cloudstorage": 1024}}
	flaky.ts = httptest.NewServer(flaky)
	defer flaky.ts.Close()
	client := newIsolateServer(flaky.ts.URL, "default-gzip", fastRetry)

	digests, contents, expected := makeItems(large)
	states, err := client.Contains(digests)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, len(digests), len(states))
	for _, state := range states {
		err = client.Push(state, bytes.NewReader(contents[state.status.Index]))
		ut.AssertEqual(t, nil, err)
	}
	ut.AssertEqual(t, expected, server.Contents())
	ut.AssertEqual(t, map[string]int{}, flaky.tearDown)

	// Look up again to confirm.
	states, err = client.Contains(digests)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, len(digests), len(states))
	for _, state := range states {
		ut.AssertEqual(t, (*PushState)(nil), state)
	}
	ut.AssertEqual(t, nil, server.Error())
}

func TestIsolateServerBadURL(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}
	client := newIsolateServer("http://asdfad.nonexistent.local", "default-gzip", fastRetry)
	caps, err := client.ServerCapabilities()
	ut.AssertEqual(t, (*isolated.ServerCapabilities)(nil), caps)
	ut.AssertEqual(t, true, err != nil)
}

// Private stuff.

var (
	cantRetry = &retry.Config{
		MaxTries:            10,
		SleepMax:            10 * time.Second,
		SleepBase:           10 * time.Second,
		SleepMultiplicative: 10 * time.Second,
	}
	fastRetry = &retry.Config{
		MaxTries:            10,
		SleepMax:            0,
		SleepBase:           0,
		SleepMultiplicative: 0,
	}

	bar   = []byte("bar")
	foo   = []byte("foo")
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

func makeItems(contents ...[]byte) ([]*isolated.DigestItem, [][]byte, map[isolated.HexDigest][]byte) {
	digests := make([]*isolated.DigestItem, 0, len(contents))
	expected := make(map[isolated.HexDigest][]byte, len(contents))
	for _, content := range contents {
		hex := isolated.HashBytes(content)
		digests = append(digests, &isolated.DigestItem{hex, false, int64(len(content))})
		expected[hex] = content
	}
	return digests, contents, expected
}

func testNormal(t *testing.T, contents ...[]byte) {
	digests, _, expected := makeItems(contents...)
	server := isolatedfake.New()
	ts := httptest.NewServer(server)
	defer ts.Close()
	client := newIsolateServer(ts.URL, "default-gzip", cantRetry)
	states, err := client.Contains(digests)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, len(digests), len(states))
	for _, state := range states {
		err = client.Push(state, bytes.NewReader(contents[state.status.Index]))
		ut.AssertEqual(t, nil, err)
	}
	ut.AssertEqual(t, expected, server.Contents())
	states, err = client.Contains(digests)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, len(digests), len(states))
	for _, state := range states {
		ut.AssertEqual(t, (*PushState)(nil), state)
	}
	ut.AssertEqual(t, nil, server.Error())
}

func testFlaky(t *testing.T, flake string) {
	server := isolatedfake.New()
	flaky := &killingMux{server: server, http503: map[string]int{flake: 10}}
	flaky.ts = httptest.NewServer(flaky)
	defer flaky.ts.Close()
	client := newIsolateServer(flaky.ts.URL, "default-gzip", fastRetry)

	digests, contents, expected := makeItems(foo, large)
	states, err := client.Contains(digests)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, len(digests), len(states))
	for _, state := range states {
		err = client.Push(state, bytes.NewReader(contents[state.status.Index]))
		ut.AssertEqual(t, nil, err)
	}
	ut.AssertEqual(t, expected, server.Contents())
	states, err = client.Contains(digests)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, len(digests), len(states))
	for _, state := range states {
		ut.AssertEqual(t, (*PushState)(nil), state)
	}
	ut.AssertEqual(t, nil, server.Error())
	ut.AssertEqual(t, map[string]int{}, flaky.http503)
}

// killingMux inserts tears down connection in the middle of a transfer.
type killingMux struct {
	lock     sync.Mutex
	server   http.Handler
	http503  map[string]int // Number of bytes should be read before returning HTTP 500.
	tearDown map[string]int // Number of bytes should be read before killing the connection.
	ts       *httptest.Server
}

func readBytes(r io.Reader, toRead int) {
	b := make([]byte, toRead)
	read := 0
	for read < toRead {
		n, _ := r.Read(b[:toRead-read])
		read += n
	}
}

func (k *killingMux) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	f := func() bool {
		k.lock.Lock()
		defer k.lock.Unlock()
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
