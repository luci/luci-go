// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolate

import (
	"io/ioutil"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/luci/luci-go/client/archiver"
	"github.com/luci/luci-go/client/internal/common"
	"github.com/luci/luci-go/client/isolatedclient"
	"github.com/luci/luci-go/client/isolatedclient/isolatedfake"
	"github.com/maruel/ut"
)

func TestReplaceVars(t *testing.T) {
	t.Parallel()

	opts := &ArchiveOptions{PathVariables: common.KeyValVars{"VAR": "wonderful"}}

	// Single replacement.
	r, err := ReplaceVariables("hello <(VAR) world", opts)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, "hello wonderful world", r)

	// Multiple replacement.
	r, err = ReplaceVariables("hello <(VAR) <(VAR) world", opts)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, "hello wonderful wonderful world", r)

	// Replacement of missing variable.
	r, err = ReplaceVariables("hello <(MISSING) world", opts)
	ut.AssertEqual(t, "no value for variable 'MISSING'", err.Error())
}

func TestArchive(t *testing.T) {
	// Create a .isolate file and archive it.
	server := isolatedfake.New()
	ts := httptest.NewServer(server)
	defer ts.Close()
	a := archiver.New(isolatedclient.New(ts.URL, "default-gzip"))

	tmpDir, err := ioutil.TempDir("", "archiver")
	ut.AssertEqual(t, nil, err)
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Fail()
		}
	}()

	opts := &ArchiveOptions{PathVariables: common.KeyValVars{"VAR": "wonderful"}}
	future := Archive(a, tmpDir, opts)
	ut.AssertEqual(t, ".", future.DisplayName())
	future.WaitForHashed()
	ut.AssertEqual(t, nil, a.Close())
	ut.AssertEqual(t, nil, server.Error())
}
