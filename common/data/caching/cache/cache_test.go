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
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/system/filesystem"

	. "github.com/smartystreets/goconvey/convey"
)

func testCache(t *testing.T, c *Cache) isolated.HexDigests {
	var expected isolated.HexDigests
	Convey(`Common tests performed on a cache of objects.`, func() {
		// c's policies must have MaxItems == 2 and MaxSize == 1024.
		td := t.TempDir()
		ctx := context.Background()

		namespace := isolatedclient.DefaultNamespace
		h := isolated.GetHash(namespace)
		fakeDigest := isolated.HexDigest("0123456789012345678901234567890123456789")
		badDigest := isolated.HexDigest("012345678901234567890123456789012345678")
		emptyContent := []byte{}
		emptyDigest := isolated.HashBytes(h, emptyContent)
		file1Content := []byte("foo")
		file1Digest := isolated.HashBytes(h, file1Content)
		file2Content := []byte("foo bar")
		file2Digest := isolated.HashBytes(h, file2Content)
		hardlinkContent := []byte("hardlink")
		hardlinkDigest := isolated.HashBytes(h, hardlinkContent)
		largeContent := bytes.Repeat([]byte("A"), 1023)
		largeDigest := isolated.HashBytes(h, largeContent)
		tooLargeContent := bytes.Repeat([]byte("A"), 1025)
		tooLargeDigest := isolated.HashBytes(h, tooLargeContent)

		So(c.Keys(), ShouldResemble, isolated.HexDigests{})

		So(c.Touch(fakeDigest), ShouldBeFalse)
		So(c.Touch(badDigest), ShouldBeFalse)

		c.Evict(fakeDigest)
		c.Evict(badDigest)

		r, err := c.Read(fakeDigest)
		So(r, ShouldBeNil)
		So(err, ShouldNotBeNil)
		r, err = c.Read(badDigest)
		So(r, ShouldBeNil)
		So(err, ShouldNotBeNil)

		// It's too large to fit in the cache.
		So(c.Add(ctx, tooLargeDigest, bytes.NewBuffer(tooLargeContent)), ShouldNotBeNil)

		// It gets discarded because it's too large.
		So(c.Add(ctx, largeDigest, bytes.NewBuffer(largeContent)), ShouldBeNil)
		So(c.Add(ctx, emptyDigest, bytes.NewBuffer(emptyContent)), ShouldBeNil)
		So(c.Add(ctx, emptyDigest, bytes.NewBuffer(emptyContent)), ShouldBeNil)
		So(c.Keys(), ShouldResemble, isolated.HexDigests{emptyDigest, largeDigest})
		c.Evict(emptyDigest)
		So(c.Keys(), ShouldResemble, isolated.HexDigests{largeDigest})
		So(c.Add(ctx, emptyDigest, bytes.NewBuffer(emptyContent)), ShouldBeNil)

		So(c.Add(ctx, file1Digest, bytes.NewBuffer(file1Content)), ShouldBeNil)
		So(c.Touch(emptyDigest), ShouldBeTrue)
		So(c.Add(ctx, file2Digest, bytes.NewBuffer(file2Content)), ShouldBeNil)

		r, err = c.Read(file1Digest)
		So(r, ShouldBeNil)
		So(err, ShouldNotBeNil)
		r, err = c.Read(file2Digest)
		So(err, ShouldBeNil)
		actual, err := ioutil.ReadAll(r)
		So(r.Close(), ShouldBeNil)
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, file2Content)

		expected = isolated.HexDigests{file2Digest, emptyDigest}
		So(c.Keys(), ShouldResemble, expected)

		dest := filepath.Join(td, "foo")
		So(c.Hardlink(fakeDigest, dest, os.FileMode(0600)), ShouldNotBeNil)
		So(c.Hardlink(badDigest, dest, os.FileMode(0600)), ShouldNotBeNil)
		So(c.Hardlink(file2Digest, dest, os.FileMode(0600)), ShouldBeNil)
		// See comment about the fact that it may or may not work.
		_ = c.Hardlink(file2Digest, dest, os.FileMode(0600))
		actual, err = ioutil.ReadFile(dest)
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, file2Content)

		dest = filepath.Join(td, "hardlink")
		So(c.AddWithHardlink(ctx, hardlinkDigest, bytes.NewBuffer(hardlinkContent), dest, os.ModePerm),
			ShouldBeNil)
		actual, err = ioutil.ReadFile(dest)
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, hardlinkContent)

		// |emptyDigest| is evicted.
		expected = isolated.HexDigests{hardlinkDigest, file2Digest}

		So(c.Close(), ShouldBeNil)
	})
	return expected
}

func TestNew(t *testing.T) {
	Convey(`Test the disk-based cache of objects.`, t, func() {
		td := t.TempDir()

		pol := Policies{MaxSize: 1024, MaxItems: 2}
		namespace := isolatedclient.DefaultNamespace
		h := isolated.GetHash(namespace)
		c, err := New(pol, td, h)
		So(err, ShouldBeNil)
		expected := testCache(t, c)

		c, err = New(pol, td, h)
		So(err, ShouldBeNil)
		So(c.Keys(), ShouldResemble, expected)
		So(c.Close(), ShouldBeNil)

		curdir, err := os.Getwd()
		So(err, ShouldBeNil)
		defer func() {
			So(os.Chdir(curdir), ShouldBeNil)
		}()

		So(os.Chdir(td), ShouldBeNil)

		rel, err := filepath.Rel(td, t.TempDir())
		So(err, ShouldBeNil)
		So(filepath.IsAbs(rel), ShouldBeFalse)
		_, err = New(pol, rel, h)
		So(err, ShouldBeNil)
	})

	Convey(`invalid state.json`, t, func() {
		dir := t.TempDir()
		state := filepath.Join(dir, "state.json")
		invalid := filepath.Join(dir, "invalid file")
		So(ioutil.WriteFile(state, []byte("invalid"), os.ModePerm), ShouldBeNil)
		So(ioutil.WriteFile(invalid, []byte("invalid"), os.ModePerm), ShouldBeNil)

		c, err := New(Policies{}, dir, isolated.GetHash(isolatedclient.DefaultNamespace))
		So(err, ShouldNotBeNil)
		if c == nil {
			t.Errorf("c should not be nil: %v", err)
		}
		So(c, ShouldNotBeNil)

		So(c.statePath(), ShouldEqual, state)

		// invalid files should be removed.
		empty, err := filesystem.IsEmptyDir(dir)
		So(err, ShouldBeNil)
		So(empty, ShouldBeTrue)

		So(c.Close(), ShouldBeNil)
	})

	Convey(`MinFreeSpace too big`, t, func() {
		ctx := context.Background()
		dir := t.TempDir()
		namespace := isolatedclient.DefaultNamespace
		h := isolated.GetHash(namespace)
		c, err := New(Policies{MaxSize: 10, MinFreeSpace: math.MaxInt64}, dir, h)
		So(err, ShouldBeNil)

		file1Content := []byte("foo")
		file1Digest := isolated.HashBytes(h, file1Content)
		So(c.Add(ctx, file1Digest, bytes.NewBuffer(file1Content)), ShouldBeNil)

		So(c.Close(), ShouldBeNil)
	})

	Convey(`MaxSize 0`, t, func() {
		ctx := context.Background()
		dir := t.TempDir()
		namespace := isolatedclient.DefaultNamespace
		h := isolated.GetHash(namespace)
		c, err := New(Policies{MaxSize: 0, MaxItems: 1}, dir, h)
		So(err, ShouldBeNil)

		file1Content := []byte("foo")
		file1Digest := isolated.HashBytes(h, file1Content)
		So(c.Add(ctx, file1Digest, bytes.NewBuffer(file1Content)), ShouldBeNil)
		So(c.Keys(), ShouldHaveLength, 1)
		So(c.Close(), ShouldBeNil)
	})

	Convey(`HardLink will update used`, t, func() {
		dir := t.TempDir()
		namespace := isolatedclient.DefaultNamespace
		h := isolated.GetHash(namespace)
		onDiskContent := []byte("on disk")
		onDiskDigest := isolated.HashBytes(h, onDiskContent)
		notOnDiskContent := []byte("not on disk")
		notOnDiskDigest := isolated.HashBytes(h, notOnDiskContent)

		c, err := New(Policies{}, dir, h)
		defer func() { So(c.Close(), ShouldBeNil) }()

		So(err, ShouldBeNil)
		So(c, ShouldNotBeNil)
		perm := os.ModePerm
		So(ioutil.WriteFile(c.itemPath(onDiskDigest), onDiskContent, perm), ShouldBeNil)

		So(c.Used(), ShouldBeEmpty)
		So(c.Hardlink(notOnDiskDigest, filepath.Join(dir, "not_on_disk"), perm), ShouldNotBeNil)
		So(c.Used(), ShouldBeEmpty)
		So(c.Hardlink(onDiskDigest, filepath.Join(dir, "on_disk"), perm), ShouldBeNil)
		So(c.Used(), ShouldHaveLength, 1)
	})

	Convey(`AddFileWithoutValidation`, t, func() {
		ctx := context.Background()
		dir := t.TempDir()
		cache := filepath.Join(dir, "cache")
		h := isolated.GetHash(isolatedclient.DefaultNamespace)

		c, err := New(Policies{
			MaxSize:  1,
			MaxItems: 1,
		}, cache, h)
		defer func() { So(c.Close(), ShouldBeNil) }()
		So(err, ShouldBeNil)

		empty := filepath.Join(dir, "empty")
		So(ioutil.WriteFile(empty, nil, 0600), ShouldBeNil)

		emptyHash := isolated.HashBytes(h, nil)

		So(c.AddFileWithoutValidation(ctx, emptyHash, empty), ShouldBeNil)

		So(c.Touch(emptyHash), ShouldBeTrue)

		// Adding already existing file is fine.
		So(c.AddFileWithoutValidation(ctx, emptyHash, empty), ShouldBeNil)

		empty2 := filepath.Join(dir, "empty2")
		So(ioutil.WriteFile(empty2, nil, 0600), ShouldBeNil)
		So(c.AddFileWithoutValidation(ctx, emptyHash, empty2), ShouldBeNil)
	})
}
