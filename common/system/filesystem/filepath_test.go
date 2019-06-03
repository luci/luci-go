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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSetReadOnly(t *testing.T) {
	t.Parallel()

	Convey("Simple tests for SetReadOnly", t, func() {
		tmpdir, err := ioutil.TempDir("", "TestSetReadOnly-")
		So(err, ShouldBeNil)
		defer func() {
			So(os.RemoveAll(tmpdir), ShouldBeNil)
		}()

		tmpfile := filepath.Join(tmpdir, "tmp")
		f, err := os.Create(tmpfile)
		So(err, ShouldBeNil)
		So(f.Close(), ShouldBeNil)

		// make tmpfile readonly
		So(SetReadOnly(tmpfile, true), ShouldBeNil)

		_, err = os.Create(tmpfile)
		So(err, ShouldNotBeNil)

		f, err = os.Open(tmpfile)
		So(err, ShouldBeNil)
		So(f.Close(), ShouldBeNil)

		// make tmpfile writable
		So(SetReadOnly(tmpfile, false), ShouldBeNil)

		f, err = os.Create(tmpfile)
		So(err, ShouldBeNil)
		So(f.Close(), ShouldBeNil)

		f, err = os.Open(tmpfile)
		So(err, ShouldBeNil)
		So(f.Close(), ShouldBeNil)
	})

}

func TestMakeTreeReadOnly(t *testing.T) {
	Convey("Simple test for MakeTreeReadOnly", t, func() {
		tmpdir, err := ioutil.TempDir("", "TestMakeTreeReadOnly-")
		So(err, ShouldBeNil)
		defer func() {
			So(os.RemoveAll(tmpdir), ShouldBeNil)
		}()

		tmpfile := filepath.Join(tmpdir, "tmp")
		f, err := os.Create(tmpfile)
		So(err, ShouldBeNil)
		So(f.Close(), ShouldBeNil)

		So(MakeTreeReadOnly(context.Background(), tmpdir), ShouldBeNil)

		_, err = os.Create(tmpfile)
		So(err, ShouldNotBeNil)

		// for os.RemoveAll
		So(SetReadOnly(tmpdir, false), ShouldBeNil)
	})
}

func TestMakeTreeFilesReadOnly(t *testing.T) {
	Convey("Simple test for MakeTreeFilesReadOnly", t, func() {
		tmpdir, err := ioutil.TempDir("", "TestMakeTreeFilesReadOnly-")
		So(err, ShouldBeNil)
		defer func() {
			So(os.RemoveAll(tmpdir), ShouldBeNil)
		}()

		tmpfile := filepath.Join(tmpdir, "tmp")
		f, err := os.Create(tmpfile)
		So(err, ShouldBeNil)
		So(f.Close(), ShouldBeNil)

		So(MakeTreeFilesReadOnly(context.Background(), tmpdir), ShouldBeNil)

		Convey("cannot change existing file", func() {
			_, err := os.Create(tmpfile)
			So(err, ShouldNotBeNil)
		})

		Convey("but it is possible to create new file", func() {
			f, err := os.Create(filepath.Join(tmpdir, "tmp2"))
			So(err, ShouldBeNil)
			So(f.Close(), ShouldBeNil)
		})
	})
}

func TestMakeTreeWritable(t *testing.T) {
	Convey("Simple test for MakeTreeWritable", t, func() {
		tmpdir, err := ioutil.TempDir("", "TestMakeTreeWritable-")
		So(err, ShouldBeNil)
		defer func() {
			So(os.RemoveAll(tmpdir), ShouldBeNil)
		}()

		tmpfile := filepath.Join(tmpdir, "tmp")
		f, err := os.Create(tmpfile)
		So(err, ShouldBeNil)
		So(f.Close(), ShouldBeNil)

		So(MakeTreeReadOnly(context.Background(), tmpdir), ShouldBeNil)

		Convey("cannot change existing file after MakeTreeReadOnly", func() {
			_, err := os.Create(tmpfile)
			So(err, ShouldNotBeNil)
		})

		So(MakeTreeWritable(context.Background(), tmpdir), ShouldBeNil)

		Convey("can change existing file after MakeTreeWritable", func() {
			f, err := os.Create(tmpfile)
			So(err, ShouldBeNil)
			So(f.Close(), ShouldBeNil)
		})
	})
}
