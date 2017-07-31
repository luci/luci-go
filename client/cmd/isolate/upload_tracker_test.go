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

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"reflect"
	"testing"

	"github.com/luci/luci-go/common/isolated"
	"github.com/luci/luci-go/common/isolatedclient"
	. "github.com/smartystreets/goconvey/convey"
)

// Fake OS imitiates a filesystem by storing file contents in a map.
// It also provides a dummy Readlink implementation.
type fakeOS struct {
	files map[string]*bytes.Buffer
}

// shouldResembleByteMap asserts that actual (a map[string]*bytes.Buffer) contains
// the same data as expected (a map[string][]byte).
// The types of actual and expected differ to make writing tests with fakeOS more convenient.
func shouldResembleByteMap(actual interface{}, expected ...interface{}) string {
	act, ok := actual.(map[string]*bytes.Buffer)
	if !ok {
		return "actual is not a map[string]*bytes.Buffer"
	}
	if len(expected) != 1 {
		return "expected is not a map[string][]byte"
	}
	exp, ok := expected[0].(map[string][]byte)
	if !ok {
		return "expected is not a map[string][]byte"
	}

	if len(act) != len(exp) {
		return fmt.Sprintf("len(actual) != len(expected): %v != %v", len(act), len(exp))
	}

	for k, v := range act {
		if got, want := v.Bytes(), exp[k]; !reflect.DeepEqual(got, want) {
			return fmt.Sprintf("actual[%q] != expected[%q]: %q != %q", k, k, got, want)
		}
	}
	return ""
}

func (fos *fakeOS) Readlink(name string) (string, error) {
	return "link:" + name, nil
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }

// implements OpenFile by returning a writer that writes to a bytes.Buffer.
func (fos *fakeOS) OpenFile(name string, flag int, perm os.FileMode) (io.WriteCloser, error) {
	if fos.files == nil {
		fos.files = make(map[string]*bytes.Buffer)
	}
	fos.files[name] = &bytes.Buffer{}
	return nopWriteCloser{fos.files[name]}, nil
}

// fakeChecker implements Checker by responding to method invocations by
// invoking the supplied callback with the supplied item, and a hard-coded *PushState.
type fakeChecker struct {
	ps *isolatedclient.PushState
}

type checkerAddItemArgs struct {
	item     *Item
	isolated bool
}

type checkerAddItemResponse struct {
	item *Item
	ps   *isolatedclient.PushState
}

func (checker *fakeChecker) AddItem(item *Item, isolated bool, callback CheckerCallback) {
	callback(item, checker.ps)
}

func (checker *fakeChecker) Close() error { return nil }

// fakeChecker implements Uploader while recording method arguments.
type fakeUploader struct {
	// uploadBytesCalls is a record of the arguments to each call of UploadBytes.
	uploadBytesCalls []uploaderUploadBytesArgs

	Uploader // TODO(mcgreevy): implement other methods.
}

type uploaderUploadBytesArgs struct {
	relPath  string
	isolJSON []byte
	ps       *isolatedclient.PushState
}

func (uploader *fakeUploader) UploadBytes(relPath string, isolJSON []byte, ps *isolatedclient.PushState, f func()) {
	uploader.uploadBytesCalls = append(uploader.uploadBytesCalls, uploaderUploadBytesArgs{relPath, isolJSON, ps})
}

func (uploader *fakeUploader) Close() error { return nil }

func TestSkipsUpload(t *testing.T) {
	t.Parallel()
	Convey(`nil push state signals that upload should be skipped`, t, func() {
		isol := &isolated.Isolated{}

		// nil PushState means skip the upload.
		checker := &fakeChecker{ps: nil}

		uploader := &fakeUploader{}

		ut := NewUploadTracker(checker, uploader, isol)
		fos := &fakeOS{}
		ut.lOS = fos // Override filesystem calls with fake.

		parts := partitionedDeps{} // no actual deps.
		err := ut.UploadDeps(parts)
		So(err, ShouldBeNil)

		// No deps, so Files should be empty.
		wantFiles := map[string]isolated.File{}
		So(ut.isol.Files, ShouldResemble, wantFiles)

		isolSummary, err := ut.Finalize("/a/isolatedPath")
		So(err, ShouldBeNil)

		// In this test, the only item that is checked and uploaded is the generated isolated file.
		wantIsolJSON := []byte(`{"algo":"","version":""}`)
		So(fos.files, shouldResembleByteMap, map[string][]byte{"/a/isolatedPath": wantIsolJSON})

		So(isolSummary.Digest, ShouldResemble, isolated.HashBytes(wantIsolJSON))
		So(isolSummary.Name, ShouldEqual, "isolatedPath")

		So(uploader.uploadBytesCalls, ShouldEqual, nil)
	})
}

func TestDontSkipUpload(t *testing.T) {
	t.Parallel()
	Convey(`passing non-nil push state signals that upload should be performed`, t, func() {
		isol := &isolated.Isolated{}

		// non-nil PushState means don't skip the upload.
		checker := &fakeChecker{ps: &isolatedclient.PushState{}}

		uploader := &fakeUploader{}

		ut := NewUploadTracker(checker, uploader, isol)
		fos := &fakeOS{}
		ut.lOS = fos // Override filesystem calls with fake.

		parts := partitionedDeps{} // no actual deps.
		err := ut.UploadDeps(parts)
		So(err, ShouldBeNil)

		// No deps, so Files should be empty.
		wantFiles := map[string]isolated.File{}
		So(ut.isol.Files, ShouldResemble, wantFiles)

		isolSummary, err := ut.Finalize("/a/isolatedPath")
		So(err, ShouldBeNil)

		// In this test, the only item that is checked and uploaded is the generated isolated file.
		wantIsolJSON := []byte(`{"algo":"","version":""}`)
		So(fos.files, shouldResembleByteMap, map[string][]byte{"/a/isolatedPath": wantIsolJSON})

		So(isolSummary.Digest, ShouldResemble, isolated.HashBytes(wantIsolJSON))
		So(isolSummary.Name, ShouldEqual, "isolatedPath")

		// Upload was not skipped.
		So(uploader.uploadBytesCalls, ShouldResemble, []uploaderUploadBytesArgs{
			{"isolatedPath", wantIsolJSON, checker.ps},
		})
	})
}

func TestHandlesSymlinks(t *testing.T) {
	t.Parallel()
	Convey(`Symlinks should be stored in the isolated json`, t, func() {
		isol := &isolated.Isolated{}

		// The checker and uploader will only be used for uploading the isolated JSON.
		// The symlinks themselves do not need to be checked or uploaded.
		// non-nil PushState means don't skip the upload.
		checker := &fakeChecker{ps: &isolatedclient.PushState{}}
		uploader := &fakeUploader{}

		ut := NewUploadTracker(checker, uploader, isol)
		fos := &fakeOS{}
		ut.lOS = fos // Override filesystem calls with fake.

		parts := partitionedDeps{
			links: itemGroup{
				items: []*Item{
					{Path: "/a/b/c", RelPath: "c"},
				},
				totalSize: 9,
			},
		}
		err := ut.UploadDeps(parts)
		So(err, ShouldBeNil)

		path := "link:/a/b/c"
		wantFiles := map[string]isolated.File{"c": {Link: &path}}

		So(ut.isol.Files, ShouldResemble, wantFiles)

		// Symlinks are not uploaded.
		So(uploader.uploadBytesCalls, ShouldEqual, nil)

		isolSummary, err := ut.Finalize("/a/isolatedPath")
		So(err, ShouldBeNil)

		// In this test, the only item that is checked and uploaded is the generated isolated file.
		wantIsolJSON := []byte(`{"algo":"","files":{"c":{"l":"link:/a/b/c"}},"version":""}`)
		So(fos.files, shouldResembleByteMap, map[string][]byte{"/a/isolatedPath": wantIsolJSON})

		So(isolSummary.Digest, ShouldResemble, isolated.HashBytes(wantIsolJSON))
		So(isolSummary.Name, ShouldEqual, "isolatedPath")

		So(uploader.uploadBytesCalls, ShouldResemble, []uploaderUploadBytesArgs{
			{"isolatedPath", wantIsolJSON, checker.ps},
		})
	})
}
