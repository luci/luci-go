// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolatedclient

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/luci/luci-go/common/isolated"
	"github.com/maruel/ut"
)

func TestMemoryCache(t *testing.T) {
	td, err := ioutil.TempDir("", "isolateserver")
	ut.AssertEqual(t, nil, err)
	defer os.RemoveAll(td)

	d := isolated.HexDigest("0123456789012345678901234567890123456789")
	c := MakeMemoryCache()
	ut.AssertEqual(t, []isolated.HexDigest{}, c.CachedSet())
	ut.AssertEqual(t, false, c.Touch(d, 0))
	c.Evict(d)
	r, err := c.Read(d)
	ut.AssertEqual(t, nil, r)
	ut.AssertEqual(t, os.ErrNotExist, err)
	empty := isolated.HexDigest(hex.EncodeToString(sha1.New().Sum(nil)))
	ut.AssertEqual(t, nil, c.Write(empty, bytes.NewBufferString("")))
	content := []byte("foo")
	h := sha1.New()
	h.Write(content)
	digest := isolated.HexDigest(hex.EncodeToString(h.Sum(nil)))
	ut.AssertEqual(t, os.ErrInvalid, c.Write(empty, bytes.NewBuffer(content)))
	ut.AssertEqual(t, nil, c.Write(digest, bytes.NewBuffer(content)))

	r, err = c.Read(digest)
	ut.AssertEqual(t, nil, err)
	actual, err := ioutil.ReadAll(r)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, content, actual)

	//[]isolated.HexDigest{digest, empty}
	ut.AssertEqual(t, 2, len(c.CachedSet()))

	dest := filepath.Join(td, "foo")
	ut.AssertEqual(t, nil, c.Hardlink(digest, dest, os.FileMode(0600)))
	actual, err = ioutil.ReadFile(dest)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, content, actual)
}
