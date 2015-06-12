// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolatedclient

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
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
	expected := map[isolated.HexDigest][]byte{
		"0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33": []byte("foo"),
		"62cdb7020ff920e5aa642c3d4066950dd1f01f4d": []byte("bar"),
	}
	testNormal(t, makeItems("foo", "bar"), expected)
}

func TestIsolateServerLarge(t *testing.T) {
	t.Parallel()
	large := strings.Repeat("0123456789abcdef", 1024*1024/16)
	expected := map[isolated.HexDigest][]byte{
		"7b961ac18d33b99122ed5d88d1ce62dd19fc8c69": []byte(large),
	}
	testNormal(t, makeItems(large), expected)
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
	flaky := &killingMux{server: server, fail: map[string]int{"/fake/cloudstorage": 1}}
	ts := httptest.NewServer(flaky)
	flaky.ts = ts
	defer ts.Close()
	client := newIsolateServer(ts.URL, "default-gzip", fastRetry)

	large := strings.Repeat("0123456789abcdef", 1024*1024/16)
	files := makeItems(large)
	expected := map[isolated.HexDigest][]byte{
		"7b961ac18d33b99122ed5d88d1ce62dd19fc8c69": []byte(large),
	}
	states, err := client.Contains(files.digests)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, len(files.digests), len(states))
	for index, state := range states {
		err = client.Push(state, bytes.NewReader(files.contents[index]))
		ut.AssertEqual(t, nil, err)
	}
	ut.AssertEqual(t, expected, server.Contents())
	states, err = client.Contains(files.digests)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, len(files.digests), len(states))
	for _, state := range states {
		ut.AssertEqual(t, (*PushState)(nil), state)
	}
	ut.AssertEqual(t, nil, server.Error())
	ut.AssertEqual(t, map[string]int{}, flaky.fail)
}

func TestIsolateServerBadURL(t *testing.T) {
	t.Parallel()
	client := newIsolateServer("http://asdfad.nonexistent", "default-gzip", fastRetry)
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
)

type items struct {
	digests  []*isolated.DigestItem
	contents [][]byte
}

func makeItems(contents ...string) items {
	out := items{}
	for _, content := range contents {
		c := []byte(content)
		hex := isolated.HashBytes(c)
		out.digests = append(out.digests, &isolated.DigestItem{hex, false, int64(len(content))})
		out.contents = append(out.contents, c)
	}
	return out
}

func testNormal(t *testing.T, files items, expected map[isolated.HexDigest][]byte) {
	server := isolatedfake.New()
	ts := httptest.NewServer(server)
	defer ts.Close()
	client := newIsolateServer(ts.URL, "default-gzip", cantRetry)
	states, err := client.Contains(files.digests)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, len(files.digests), len(states))
	for index, state := range states {
		err = client.Push(state, bytes.NewReader(files.contents[index]))
		ut.AssertEqual(t, nil, err)
	}
	ut.AssertEqual(t, expected, server.Contents())
	states, err = client.Contains(files.digests)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, len(files.digests), len(states))
	for _, state := range states {
		ut.AssertEqual(t, (*PushState)(nil), state)
	}
	ut.AssertEqual(t, nil, server.Error())
}

func testFlaky(t *testing.T, flake string) {
	server := isolatedfake.New()
	flaky := &failingMux{server: server, fail: map[string]int{flake: 1}}
	ts := httptest.NewServer(flaky)
	defer ts.Close()
	client := newIsolateServer(ts.URL, "default-gzip", fastRetry)

	large := strings.Repeat("0123456789abcdef", 1024*1024/16)
	files := makeItems("foo", large)
	expected := map[isolated.HexDigest][]byte{
		"0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33": []byte("foo"),
		"7b961ac18d33b99122ed5d88d1ce62dd19fc8c69": []byte(large),
	}
	states, err := client.Contains(files.digests)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, len(files.digests), len(states))
	for index, state := range states {
		err = client.Push(state, bytes.NewReader(files.contents[index]))
		ut.AssertEqual(t, nil, err)
	}
	ut.AssertEqual(t, expected, server.Contents())
	states, err = client.Contains(files.digests)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, len(files.digests), len(states))
	for _, state := range states {
		ut.AssertEqual(t, (*PushState)(nil), state)
	}
	ut.AssertEqual(t, nil, server.Error())
	ut.AssertEqual(t, map[string]int{}, flaky.fail)
}

// failingMux inserts HTTP 500 in the urls specified.
type failingMux struct {
	lock   sync.Mutex
	server http.Handler
	fail   map[string]int // Number of times each path should return HTTP 500.
}

func (f *failingMux) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	f.lock.Lock()
	if v, ok := f.fail[req.URL.Path]; ok {
		v--
		if v == 0 {
			delete(f.fail, req.URL.Path)
		} else {
			f.fail[req.URL.Path] = v
		}
		f.lock.Unlock()
		w.WriteHeader(500)
		return
	}
	f.lock.Unlock()
	f.server.ServeHTTP(w, req)
}

// killingMux inserts tears down connection in the middle of a transfer.
type killingMux struct {
	lock   sync.Mutex
	server http.Handler
	fail   map[string]int // Number of times each path should cut after 1kb.
	ts     *httptest.Server
}

func (k *killingMux) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	k.lock.Lock()
	if v, ok := k.fail[req.URL.Path]; ok {
		v--
		if v == 0 {
			delete(k.fail, req.URL.Path)
		} else {
			k.fail[req.URL.Path] = v
		}
		// Kills all TCP connection to simulate a broken server.
		k.ts.CloseClientConnections()
		k.lock.Unlock()
		return
	}
	k.lock.Unlock()
	k.server.ServeHTTP(w, req)
}
