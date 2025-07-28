// Copyright 2019 The LUCI Authors.
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

package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSetReadOnly(t *testing.T) {
	t.Parallel()

	ftt.Run("Simple tests for SetReadOnly", t, func(t *ftt.Test) {
		tmpdir := t.TempDir()

		tmpfile := filepath.Join(tmpdir, "tmp")
		f, err := os.Create(tmpfile)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, f.Close(), should.BeNil)

		// make tmpfile readonly
		assert.Loosely(t, SetReadOnly(tmpfile, true), should.BeNil)

		_, err = os.Create(tmpfile)
		assert.Loosely(t, err, should.NotBeNil)

		f, err = os.Open(tmpfile)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, f.Close(), should.BeNil)

		// make tmpfile writable
		assert.Loosely(t, SetReadOnly(tmpfile, false), should.BeNil)

		f, err = os.Create(tmpfile)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, f.Close(), should.BeNil)

		f, err = os.Open(tmpfile)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, f.Close(), should.BeNil)
	})
}

func TestMakeTreeReadOnly(t *testing.T) {
	ftt.Run("Simple test for MakeTreeReadOnly", t, func(t *ftt.Test) {
		tmpdir := t.TempDir()

		tmpfile := filepath.Join(tmpdir, "tmp")
		f, err := os.Create(tmpfile)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, f.Close(), should.BeNil)

		assert.Loosely(t, MakeTreeReadOnly(context.Background(), tmpdir), should.BeNil)

		_, err = os.Create(tmpfile)
		assert.Loosely(t, err, should.NotBeNil)

		// for os.RemoveAll
		assert.Loosely(t, SetReadOnly(tmpdir, false), should.BeNil)
	})
}

func TestMakeTreeFilesReadOnly(t *testing.T) {
	ftt.Run("Simple test for MakeTreeFilesReadOnly", t, func(t *ftt.Test) {
		tmpdir := t.TempDir()

		tmpfile := filepath.Join(tmpdir, "tmp")
		f, err := os.Create(tmpfile)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, f.Close(), should.BeNil)

		assert.Loosely(t, MakeTreeFilesReadOnly(context.Background(), tmpdir), should.BeNil)

		t.Run("cannot change existing file", func(t *ftt.Test) {
			_, err := os.Create(tmpfile)
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("but it is possible to create new file", func(t *ftt.Test) {
			f, err := os.Create(filepath.Join(tmpdir, "tmp2"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, f.Close(), should.BeNil)
		})
	})
}

func TestMakeTreeWritable(t *testing.T) {
	ftt.Run("Simple test for MakeTreeWritable", t, func(t *ftt.Test) {
		tmpdir := t.TempDir()

		tmpfile := filepath.Join(tmpdir, "tmp")
		f, err := os.Create(tmpfile)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, f.Close(), should.BeNil)

		assert.Loosely(t, MakeTreeReadOnly(context.Background(), tmpdir), should.BeNil)

		t.Run("cannot change existing file after MakeTreeReadOnly", func(t *ftt.Test) {
			_, err := os.Create(tmpfile)
			assert.Loosely(t, err, should.NotBeNil)
		})

		assert.Loosely(t, MakeTreeWritable(context.Background(), tmpdir), should.BeNil)

		t.Run("can change existing file after MakeTreeWritable", func(t *ftt.Test) {
			f, err := os.Create(tmpfile)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, f.Close(), should.BeNil)
		})
	})
}

func TestResolveSymlink(t *testing.T) {
	t.Parallel()
	ftt.Run("ResolveSymlink", t, func(t *ftt.Test) {
		dir := t.TempDir()
		regular, err := os.Create(filepath.Join(dir, "regular"))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, regular.Close(), should.BeNil)

		assert.Loosely(t, os.Symlink("regular", filepath.Join(dir, "symlink1")), should.BeNil)
		assert.Loosely(t, os.Symlink("symlink1", filepath.Join(dir, "symlink2")), should.BeNil)
		assert.Loosely(t, os.Symlink("symlink2", filepath.Join(dir, "symlink3")), should.BeNil)

		path, _, err := ResolveSymlink(filepath.Join(dir, "symlink3"))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, path, should.Equal(filepath.Join(dir, "regular")))
	})
}

func TestGetFilenameNoExt(t *testing.T) {
	t.Parallel()
	ftt.Run("Basic", t, func(t *ftt.Test) {
		p := filepath.Join("foo", "bar.txt")
		name := GetFilenameNoExt(p)
		assert.Loosely(t, name, should.Match("bar"))

		p = filepath.Join("foo", "bar", "baz")
		name = GetFilenameNoExt(p)
		assert.Loosely(t, name, should.Match("baz"))

		p = filepath.Join("foo.bar.baz")
		name = GetFilenameNoExt(p)
		assert.Loosely(t, name, should.Match("foo.bar"))
	})
}
