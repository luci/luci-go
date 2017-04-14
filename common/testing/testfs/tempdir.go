// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package testfs

import (
	"testing"

	"github.com/luci/luci-go/common/system/filesystem"
)

// WithTempDir creates a temporary directory and passes it to fn. After fn
// exits, the directory is cleaned up.
func WithTempDir(t *testing.T, prefix string, fn func(string) error) error {
	td := filesystem.TempDir{
		Prefix: prefix,
		CleanupErrFunc: func(tdir string, err error) {
			t.Errorf("failed to remove temporary directory [%s]: %s", tdir, err)
		},
	}
	return td.With(fn)
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
