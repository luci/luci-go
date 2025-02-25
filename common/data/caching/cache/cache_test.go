// Copyright 2015 The LUCI Authors.
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

package cache

import (
	"bytes"
	"context"
	"crypto"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/system/filesystem"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func testCache(t testing.TB, c *Cache) HexDigests {
	t.Helper()

	var expected HexDigests
	// c's policies must have MaxItems == 2 and MaxSize == 1024.
	td := t.TempDir()
	ctx := context.Background()

	h := crypto.SHA1
	fakeDigest := HexDigest("0123456789012345678901234567890123456789")
	badDigest := HexDigest("012345678901234567890123456789012345678")
	emptyContent := []byte{}
	emptyDigest := HashBytes(h, emptyContent)
	file1Content := []byte("foo")
	file1Digest := HashBytes(h, file1Content)
	file2Content := []byte("foo bar")
	file2Digest := HashBytes(h, file2Content)
	hardlinkContent := []byte("hardlink")
	hardlinkDigest := HashBytes(h, hardlinkContent)
	largeContent := bytes.Repeat([]byte("A"), 1023)
	largeDigest := HashBytes(h, largeContent)
	tooLargeContent := bytes.Repeat([]byte("A"), 1025)
	tooLargeDigest := HashBytes(h, tooLargeContent)

	assert.Loosely(t, c.Keys(), should.Match(HexDigests{}), truth.LineContext())

	assert.Loosely(t, c.Touch(fakeDigest), should.BeFalse, truth.LineContext())
	assert.Loosely(t, c.Touch(badDigest), should.BeFalse, truth.LineContext())

	c.Evict(fakeDigest)
	c.Evict(badDigest)

	r, err := c.Read(fakeDigest)
	assert.Loosely(t, r, should.BeNil, truth.LineContext())
	assert.Loosely(t, err, should.NotBeNil, truth.LineContext())
	r, err = c.Read(badDigest)
	assert.Loosely(t, r, should.BeNil, truth.LineContext())
	assert.Loosely(t, err, should.NotBeNil, truth.LineContext())

	// It's too large to fit in the cache.
	assert.Loosely(t, c.Add(ctx, tooLargeDigest, bytes.NewBuffer(tooLargeContent)), should.NotBeNil, truth.LineContext())

	// It gets discarded because it's too large.
	assert.Loosely(t, c.Add(ctx, largeDigest, bytes.NewBuffer(largeContent)), should.BeNil, truth.LineContext(), truth.LineContext())
	assert.Loosely(t, c.Add(ctx, emptyDigest, bytes.NewBuffer(emptyContent)), should.BeNil, truth.LineContext())
	assert.Loosely(t, c.Add(ctx, emptyDigest, bytes.NewBuffer(emptyContent)), should.BeNil, truth.LineContext())
	assert.Loosely(t, c.Keys(), should.Match(HexDigests{emptyDigest, largeDigest}), truth.LineContext())
	c.Evict(emptyDigest)
	assert.Loosely(t, c.Keys(), should.Match(HexDigests{largeDigest}), truth.LineContext())
	assert.Loosely(t, c.Add(ctx, emptyDigest, bytes.NewBuffer(emptyContent)), should.BeNil, truth.LineContext())

	assert.Loosely(t, c.Add(ctx, file1Digest, bytes.NewBuffer(file1Content)), should.BeNil, truth.LineContext())
	assert.Loosely(t, c.Touch(emptyDigest), should.BeTrue, truth.LineContext())
	assert.Loosely(t, c.Add(ctx, file2Digest, bytes.NewBuffer(file2Content)), should.BeNil, truth.LineContext())

	r, err = c.Read(file1Digest)
	assert.Loosely(t, r, should.BeNil, truth.LineContext())
	assert.Loosely(t, err, should.NotBeNil, truth.LineContext())
	r, err = c.Read(file2Digest)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	actual, err := io.ReadAll(r)
	assert.Loosely(t, r.Close(), should.BeNil, truth.LineContext())
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	assert.Loosely(t, actual, should.Match(file2Content), truth.LineContext())

	expected = HexDigests{file2Digest, emptyDigest}
	assert.Loosely(t, c.Keys(), should.Match(expected), truth.LineContext())

	dest := filepath.Join(td, "foo")
	assert.Loosely(t, c.Hardlink(fakeDigest, dest, os.FileMode(0600)), should.NotBeNil, truth.LineContext())
	assert.Loosely(t, c.Hardlink(badDigest, dest, os.FileMode(0600)), should.NotBeNil, truth.LineContext())
	assert.Loosely(t, c.Hardlink(file2Digest, dest, os.FileMode(0600)), should.BeNil, truth.LineContext())
	// See comment about the fact that it may or may not work.
	_ = c.Hardlink(file2Digest, dest, os.FileMode(0600))
	actual, err = os.ReadFile(dest)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	assert.Loosely(t, actual, should.Match(file2Content), truth.LineContext())

	dest = filepath.Join(td, "hardlink")
	assert.Loosely(t, c.AddWithHardlink(ctx, hardlinkDigest, bytes.NewBuffer(hardlinkContent), dest, os.ModePerm),
		should.BeNil, truth.LineContext())
	actual, err = os.ReadFile(dest)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	assert.Loosely(t, actual, should.Match(hardlinkContent), truth.LineContext())

	// |emptyDigest| is evicted.
	expected = HexDigests{hardlinkDigest, file2Digest}

	assert.Loosely(t, c.Close(), should.BeNil, truth.LineContext())

	return expected
}

func TestNew(t *testing.T) {
	ftt.Run(`Test the disk-based cache of objects.`, t, func(t *ftt.Test) {
		td := t.TempDir()

		pol := Policies{MaxSize: 1024, MaxItems: 2}
		h := crypto.SHA1
		c, err := New(pol, td, h)
		assert.Loosely(t, err, should.BeNil)
		expected := testCache(t, c)

		c, err = New(pol, td, h)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, c.Keys(), should.Match(expected))
		assert.Loosely(t, c.Close(), should.BeNil)

		curdir, err := os.Getwd()
		assert.Loosely(t, err, should.BeNil)
		defer func() {
			assert.Loosely(t, os.Chdir(curdir), should.BeNil)
		}()

		assert.Loosely(t, os.Chdir(td), should.BeNil)

		rel, err := filepath.Rel(td, t.TempDir())
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, filepath.IsAbs(rel), should.BeFalse)
		_, err = New(pol, rel, h)
		assert.Loosely(t, err, should.BeNil)
	})

	ftt.Run(`invalid state.json`, t, func(t *ftt.Test) {
		dir := t.TempDir()
		state := filepath.Join(dir, "state.json")
		invalid := filepath.Join(dir, "invalid file")
		assert.Loosely(t, os.WriteFile(state, []byte("invalid"), os.ModePerm), should.BeNil)
		assert.Loosely(t, os.WriteFile(invalid, []byte("invalid"), os.ModePerm), should.BeNil)

		c, err := New(Policies{}, dir, crypto.SHA1)
		assert.Loosely(t, err, should.NotBeNil)
		if c == nil {
			t.Errorf("c should not be nil: %v", err)
		}
		assert.Loosely(t, c, should.NotBeNil)

		assert.Loosely(t, c.statePath(), should.Equal(state))

		// invalid files should be removed.
		empty, err := filesystem.IsEmptyDir(dir)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, empty, should.BeTrue)

		assert.Loosely(t, c.Close(), should.BeNil)
	})

	ftt.Run(`MinFreeSpace too big`, t, func(t *ftt.Test) {
		ctx := context.Background()
		dir := t.TempDir()
		h := crypto.SHA1
		c, err := New(Policies{MaxSize: 10, MinFreeSpace: math.MaxInt64}, dir, h)
		assert.Loosely(t, err, should.BeNil)

		file1Content := []byte("foo")
		file1Digest := HashBytes(h, file1Content)
		assert.Loosely(t, c.Add(ctx, file1Digest, bytes.NewBuffer(file1Content)), should.BeNil)

		assert.Loosely(t, c.Close(), should.BeNil)
	})

	ftt.Run(`MaxSize 0`, t, func(t *ftt.Test) {
		ctx := context.Background()
		dir := t.TempDir()
		h := crypto.SHA1
		c, err := New(Policies{MaxSize: 0, MaxItems: 1}, dir, h)
		assert.Loosely(t, err, should.BeNil)

		file1Content := []byte("foo")
		file1Digest := HashBytes(h, file1Content)
		assert.Loosely(t, c.Add(ctx, file1Digest, bytes.NewBuffer(file1Content)), should.BeNil)
		assert.Loosely(t, c.Keys(), should.HaveLength(1))
		assert.Loosely(t, c.Close(), should.BeNil)
	})

	ftt.Run(`HardLink will update used`, t, func(t *ftt.Test) {
		dir := t.TempDir()
		h := crypto.SHA1
		onDiskContent := []byte("on disk")
		onDiskDigest := HashBytes(h, onDiskContent)
		notOnDiskContent := []byte("not on disk")
		notOnDiskDigest := HashBytes(h, notOnDiskContent)

		c, err := New(Policies{}, dir, h)
		defer func() { assert.Loosely(t, c.Close(), should.BeNil) }()

		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, c, should.NotBeNil)
		perm := os.ModePerm
		assert.Loosely(t, os.WriteFile(c.itemPath(onDiskDigest), onDiskContent, perm), should.BeNil)

		assert.Loosely(t, c.Used(), should.BeEmpty)
		assert.Loosely(t, c.Hardlink(notOnDiskDigest, filepath.Join(dir, "not_on_disk"), perm), should.NotBeNil)
		assert.Loosely(t, c.Used(), should.BeEmpty)
		assert.Loosely(t, c.Hardlink(onDiskDigest, filepath.Join(dir, "on_disk"), perm), should.BeNil)
		assert.Loosely(t, c.Used(), should.HaveLength(1))
	})

	ftt.Run(`AddFileWithoutValidation`, t, func(t *ftt.Test) {
		ctx := context.Background()
		dir := t.TempDir()
		cache := filepath.Join(dir, "cache")
		h := crypto.SHA1
		c, err := New(Policies{
			MaxSize:  1,
			MaxItems: 1,
		}, cache, h)
		defer func() { assert.Loosely(t, c.Close(), should.BeNil) }()
		assert.Loosely(t, err, should.BeNil)

		empty := filepath.Join(dir, "empty")
		assert.Loosely(t, os.WriteFile(empty, nil, 0600), should.BeNil)

		emptyHash := HashBytes(h, nil)

		assert.Loosely(t, c.AddFileWithoutValidation(ctx, emptyHash, empty), should.BeNil)

		assert.Loosely(t, c.Touch(emptyHash), should.BeTrue)

		// Adding already existing file is fine.
		assert.Loosely(t, c.AddFileWithoutValidation(ctx, emptyHash, empty), should.BeNil)

		empty2 := filepath.Join(dir, "empty2")
		assert.Loosely(t, os.WriteFile(empty2, nil, 0600), should.BeNil)
		assert.Loosely(t, c.AddFileWithoutValidation(ctx, emptyHash, empty2), should.BeNil)
	})
}
