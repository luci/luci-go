// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package archiver

import (
	"io/ioutil"
	"log"
	"net/http/httptest"
	"testing"

	"github.com/luci/luci-go/client/isolatedclient"
	"github.com/luci/luci-go/client/isolatedclient/isolatedfake"
	"github.com/luci/luci-go/common/isolated"
	"github.com/maruel/ut"
)

func init() {
	log.SetOutput(ioutil.Discard)
}

func TestArchiverEmpty(t *testing.T) {
	t.Parallel()
	a := New(isolatedclient.New("https://localhost:1", "default"))
	ut.AssertEqual(t, &Stats{[]int64{}, []int64{}, []*UploadStat{}}, a.Stats())
}

func TestArchiverFile(t *testing.T) {
	t.Parallel()
	server := isolatedfake.New(t)
	ts := httptest.NewServer(server)
	defer ts.Close()
	a := New(isolatedclient.New(ts.URL, "default"))

	fEmpty, err := ioutil.TempFile("", "archiver")
	ut.AssertEqual(t, nil, err)
	a.PushFile(fEmpty.Name())
	fFoo, err := ioutil.TempFile("", "archiver")
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, nil, ioutil.WriteFile(fFoo.Name(), []byte("foo"), 0600))
	a.PushFile(fFoo.Name())
	ut.AssertEqual(t, nil, a.Close())

	stats := a.Stats()
	ut.AssertEqual(t, []int64{}, stats.Hits)
	ut.AssertEqual(t, []int64{0, 3}, stats.Misses)
	expected := map[isolated.HexDigest][]byte{
		"0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33": []byte("foo"),
		"da39a3ee5e6b4b0d3255bfef95601890afd80709": {},
	}
	ut.AssertEqual(t, expected, server.Contents())
}
