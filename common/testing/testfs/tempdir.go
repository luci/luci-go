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
