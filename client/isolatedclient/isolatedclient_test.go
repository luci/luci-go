// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolatedclient

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/luci/luci-go/client/isolatedclient/isolatedfake"
	"github.com/luci/luci-go/common/isolated"
	"github.com/maruel/ut"
)

func TestIsolateServerCaps(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(isolatedfake.New(t))
	defer ts.Close()
	client := New(ts.URL, "default")
	caps, err := client.ServerCapabilities()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, &isolated.ServerCapabilities{"v1"}, caps)
}

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

func TestIsolateServer(t *testing.T) {
	t.Parallel()
	server := isolatedfake.New(t)
	ts := httptest.NewServer(server)
	defer ts.Close()
	client := New(ts.URL, "default")

	files := makeItems("foo", "bar")
	states, err := client.Contains(files.digests)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, len(files.digests), len(states))
	for index, state := range states {
		err = client.Push(state, bytes.NewBuffer(files.contents[index]))
		ut.AssertEqual(t, nil, err)
	}
	// foo and bar.
	expected := map[isolated.HexDigest][]byte{
		"0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33": {0x66, 0x6f, 0x6f},
		"62cdb7020ff920e5aa642c3d4066950dd1f01f4d": {0x62, 0x61, 0x72},
	}
	ut.AssertEqual(t, expected, server.Contents())
	states, err = client.Contains(files.digests)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, len(files.digests), len(states))
	for _, state := range states {
		ut.AssertEqual(t, (*PushState)(nil), state)
	}
}
