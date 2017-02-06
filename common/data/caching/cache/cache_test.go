// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package cache

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/luci/luci-go/common/isolated"

	. "github.com/smartystreets/goconvey/convey"
)

func testCache(t *testing.T, c Cache) isolated.HexDigests {
	var expected isolated.HexDigests
	Convey(`Common tests performed on a cache of objects.`, func() {
		// c's policies must have MaxItems == 2 and MaxSize == 1024.
		td, err := ioutil.TempDir("", "cache")
		So(err, ShouldBeNil)
		defer func() {
			if err := os.RemoveAll(td); err != nil {
				t.Fail()
			}
		}()

		fakeDigest := isolated.HexDigest("0123456789012345678901234567890123456789")
		badDigest := isolated.HexDigest("012345678901234567890123456789012345678")
		emptyContent := []byte{}
		emptyDigest := isolated.HashBytes(emptyContent)
		file1Content := []byte("foo")
		file1Digest := isolated.HashBytes(file1Content)
		file2Content := []byte("foo bar")
		file2Digest := isolated.HashBytes(file2Content)
		largeContent := bytes.Repeat([]byte("A"), 1023)
		largeDigest := isolated.HashBytes(largeContent)
		tooLargeContent := bytes.Repeat([]byte("A"), 1025)
		tooLargeDigest := isolated.HashBytes(tooLargeContent)

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
		So(c.Add(tooLargeDigest, bytes.NewBuffer(tooLargeContent)), ShouldNotBeNil)

		// It gets discarded because it's too large.
		So(c.Add(largeDigest, bytes.NewBuffer(largeContent)), ShouldBeNil)
		So(c.Add(emptyDigest, bytes.NewBuffer(emptyContent)), ShouldBeNil)
		So(c.Add(emptyDigest, bytes.NewBuffer(emptyContent)), ShouldBeNil)
		So(c.Keys(), ShouldResemble, isolated.HexDigests{emptyDigest, largeDigest})
		c.Evict(emptyDigest)
		So(c.Keys(), ShouldResemble, isolated.HexDigests{largeDigest})
		So(c.Add(emptyDigest, bytes.NewBuffer(emptyContent)), ShouldBeNil)

		So(c.Add(file1Digest, bytes.NewBuffer(file1Content)), ShouldBeNil)
		So(c.Touch(emptyDigest), ShouldBeTrue)
		So(c.Add(file2Digest, bytes.NewBuffer(file2Content)), ShouldBeNil)

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

		So(c.Close(), ShouldBeNil)
	})
	return expected
}

func TestNewMemory(t *testing.T) {
	Convey(`Test the memory-based cache of objects.`, t, func() {
		testCache(t, NewMemory(Policies{MaxSize: 1024, MaxItems: 2}))
	})
}

func TestNewDisk(t *testing.T) {
	Convey(`Test the disk-based cache of objects.`, t, func() {
		td, err := ioutil.TempDir("", "cache")
		So(err, ShouldBeNil)
		defer func() {
			if err := os.RemoveAll(td); err != nil {
				t.Error(err)
			}
		}()
		pol := Policies{MaxSize: 1024, MaxItems: 2}
		c, err := NewDisk(pol, td)
		So(err, ShouldBeNil)
		expected := testCache(t, c)

		c, err = NewDisk(pol, td)
		So(err, ShouldBeNil)
		So(c.Keys(), ShouldResemble, expected)
		So(c.Close(), ShouldBeNil)

		c, err = NewDisk(pol, "non absolute path")
		So(c, ShouldBeNil)
		So(err, ShouldNotBeNil)
	})
}
