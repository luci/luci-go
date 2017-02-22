// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package testfs

import (
	"io/ioutil"
	"testing"

	"github.com/luci/luci-go/vpython/filesystem"
)

// WithTempDir creates a temporary directory and passes it to fn. After fn
// exits, the directory is cleaned up.
func WithTempDir(t *testing.T, prefix string, fn func(string) error) error {
	tdir, err := ioutil.TempDir("", prefix)
	if err != nil {
		return err
	}
	defer func() {
		if rmErr := filesystem.RemoveAll(tdir); rmErr != nil {
			t.Errorf("failed to remove temporary directory [%s]: %s", tdir, rmErr)
		}
	}()
	return fn(tdir)
}

// MustWithTempDir calls WithTempDir and panics if any failures occur.
func MustWithTempDir(t *testing.T, prefix string, fn func(string)) func() {
	return func() {
		err := WithTempDir(t, prefix, func(tdir string) error {
			fn(tdir)
			return nil
		})
		if err != nil {
			panic(err)
		}
	}
}
