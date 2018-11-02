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

package archiver

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
)

// Fake OS imitiates a filesystem by storing file contents in a map.
// It also provides a dummy Readlink implementation.
type fakeOS struct {
	writeFiles map[string]*bytes.Buffer
	readFiles  map[string]io.Reader
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
	if fos.writeFiles == nil {
		fos.writeFiles = make(map[string]*bytes.Buffer)
	}
	fos.writeFiles[name] = &bytes.Buffer{}
	return nopWriteCloser{fos.writeFiles[name]}, nil
}

// implements Open by returning a pre-configured Reader.
func (fos *fakeOS) Open(name string) (io.ReadCloser, error) {
	r, ok := fos.readFiles[name]
	if !ok {
		panic(fmt.Sprintf("fakeOS: file not found (%s); not implemented.", name))
	}
	return ioutil.NopCloser(r), nil
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

func (checker *fakeChecker) PresumeExists(item *Item) {}

func (checker *fakeChecker) Close() error { return nil }

// fakeChecker implements Uploader while recording method arguments.
type fakeUploader struct {
	// uploadBytesCalls is a record of the arguments to each call of UploadBytes.
	uploadBytesCalls []uploaderUploadBytesArgs
	// uploadFileCalls is a record of the arguments to each call of UploadFile.
	uploadFileCalls []uploaderUploadFileArgs

	Uploader // TODO(mcgreevy): implement other methods.
}

type uploaderUploadBytesArgs struct {
	relPath  string
	isolJSON []byte
	ps       *isolatedclient.PushState
}

type uploaderUploadFileArgs struct {
	item *Item
	ps   *isolatedclient.PushState
}

func (uploader *fakeUploader) UploadBytes(relPath string, isolJSON []byte, ps *isolatedclient.PushState, done func()) {
	uploader.uploadBytesCalls = append(uploader.uploadBytesCalls, uploaderUploadBytesArgs{relPath, isolJSON, ps})
	done()
}

func (uploader *fakeUploader) UploadFile(item *Item, ps *isolatedclient.PushState, done func()) {
	uploader.uploadFileCalls = append(uploader.uploadFileCalls, uploaderUploadFileArgs{item, ps})
	done()
}
func (uploader *fakeUploader) Close() error { return nil }

func TestSkipsUpload(t *testing.T) {
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
		So(fos.writeFiles, shouldResembleByteMap, map[string][]byte{"/a/isolatedPath": wantIsolJSON})

		So(isolSummary.Digest, ShouldResemble, isolated.HashBytes(wantIsolJSON))
		So(isolSummary.Name, ShouldEqual, "isolatedPath")

		So(uploader.uploadBytesCalls, ShouldEqual, nil)
	})
}

func TestDontSkipUpload(t *testing.T) {
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
		So(fos.writeFiles, shouldResembleByteMap, map[string][]byte{"/a/isolatedPath": wantIsolJSON})

		So(isolSummary.Digest, ShouldResemble, isolated.HashBytes(wantIsolJSON))
		So(isolSummary.Name, ShouldEqual, "isolatedPath")

		// Upload was not skipped.
		So(uploader.uploadBytesCalls, ShouldResemble, []uploaderUploadBytesArgs{
			{"isolatedPath", wantIsolJSON, checker.ps},
		})
	})
}

func TestHandlesSymlinks(t *testing.T) {
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
		So(fos.writeFiles, shouldResembleByteMap, map[string][]byte{"/a/isolatedPath": wantIsolJSON})

		So(isolSummary.Digest, ShouldResemble, isolated.HashBytes(wantIsolJSON))
		So(isolSummary.Name, ShouldEqual, "isolatedPath")

		So(uploader.uploadBytesCalls, ShouldResemble, []uploaderUploadBytesArgs{
			{"isolatedPath", wantIsolJSON, checker.ps},
		})
	})
}

func TestHandlesIndividualFiles(t *testing.T) {
	Convey(`Individual files should be stored in the isolated json and uploaded`, t, func() {
		isol := &isolated.Isolated{}

		// non-nil PushState means don't skip the upload.
		pushState := &isolatedclient.PushState{}
		checker := &fakeChecker{ps: pushState}
		uploader := &fakeUploader{}

		ut := NewUploadTracker(checker, uploader, isol)
		fos := &fakeOS{
			readFiles: map[string]io.Reader{
				"/a/b/foo": strings.NewReader("foo contents"),
				"/a/b/bar": strings.NewReader("bar contents"),
			},
		}
		ut.lOS = fos // Override filesystem calls with fake.

		parts := partitionedDeps{
			indivFiles: itemGroup{
				items: []*Item{
					{Path: "/a/b/foo", RelPath: "foo", Size: 1, Mode: 004},
					{Path: "/a/b/bar", RelPath: "bar", Size: 2, Mode: 006},
				},
				totalSize: 3,
			},
		}
		err := ut.UploadDeps(parts)
		So(err, ShouldBeNil)

		fooHash := isolated.HashBytes([]byte("foo contents"))
		barHash := isolated.HashBytes([]byte("bar contents"))
		wantFiles := map[string]isolated.File{
			"foo": {
				Digest: fooHash,
				Mode:   4,
				Size:   Int64(1)},
			"bar": {
				Digest: barHash,
				Mode:   6,
				Size:   Int64(2)},
		}

		So(ut.isol.Files, ShouldResemble, wantFiles)

		So(uploader.uploadBytesCalls, ShouldEqual, nil)

		isolSummary, err := ut.Finalize("/a/isolatedPath")
		So(err, ShouldBeNil)

		wantIsolJSONTmpl := `{"algo":"","files":{"bar":{"h":"%s","m":6,"s":2},"foo":{"h":"%s","m":4,"s":1}},"version":""}`
		wantIsolJSON := []byte(fmt.Sprintf(wantIsolJSONTmpl, barHash, fooHash))
		So(fos.writeFiles, shouldResembleByteMap, map[string][]byte{"/a/isolatedPath": wantIsolJSON})

		So(isolSummary.Digest, ShouldResemble, isolated.HashBytes(wantIsolJSON))
		So(isolSummary.Name, ShouldEqual, "isolatedPath")

		So(uploader.uploadBytesCalls, ShouldResemble, []uploaderUploadBytesArgs{
			{"isolatedPath", wantIsolJSON, checker.ps},
		})
		So(uploader.uploadFileCalls, ShouldResemble, []uploaderUploadFileArgs{
			{
				item: &Item{
					Path:    "/a/b/foo",
					RelPath: "foo",
					Size:    1,
					Mode:    os.FileMode(4),
					Digest:  fooHash,
				},
				ps: pushState,
			},
			{
				item: &Item{
					Path:    "/a/b/bar",
					RelPath: "bar",
					Size:    2,
					Mode:    os.FileMode(6),
					Digest:  barHash,
				},
				ps: pushState,
			},
		})
	})
}

// Int is a helper routine that allocates a new int value to store v and returns a pointer to it.
func Int(i int) *int { return &i }

// Int64 is a helper routine that allocates a new int64 value to store v and returns a pointer to it.
func Int64(i int64) *int64 { return &i }
