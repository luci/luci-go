// Copyright 2014 The LUCI Authors.
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

package local

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/cipd/common"
)

func normalizeJSON(s string) (string, error) {
	// Round trip through default json marshaller to normalize indentation.
	var x map[string]interface{}
	if err := json.Unmarshal([]byte(s), &x); err != nil {
		return "", err
	}
	blob, err := json.Marshal(x)
	if err != nil {
		return "", err
	}
	return string(blob), nil
}

func shouldBeSameJSONDict(actual interface{}, expected ...interface{}) string {
	if len(expected) != 1 {
		return "Too many argument for shouldBeSameJSONDict"
	}
	actualNorm, err := normalizeJSON(actual.(string))
	if err != nil {
		return err.Error()
	}
	expectedNorm, err := normalizeJSON(expected[0].(string))
	if err != nil {
		return err.Error()
	}
	return ShouldEqual(actualNorm, expectedNorm)
}

type bytesInstanceFile struct {
	*bytes.Reader
}

func (bytesInstanceFile) Close(context.Context, bool) error { return nil }

func bytesFile(data []byte) InstanceFile {
	return bytesInstanceFile{bytes.NewReader(data)}
}

func TestPackageReading(t *testing.T) {
	ctx := context.Background()

	Convey("Open empty package works", t, func() {
		// Build an empty package.
		out := bytes.Buffer{}
		err := BuildInstance(ctx, BuildInstanceOptions{
			Output:           &out,
			PackageName:      "testing",
			CompressionLevel: 5,
		})
		So(err, ShouldBeNil)

		// Open it.
		inst, err := OpenInstance(ctx, bytesFile(out.Bytes()), "", VerifyHash)
		So(inst, ShouldNotBeNil)
		So(err, ShouldBeNil)
		So(inst.Pin(), ShouldResemble, Pin{"testing", "b1b76479c47e4ea4da7b6f4629c0d58ff7dc6569"})
		So(len(inst.Files()), ShouldEqual, 1)

		// Contains single manifest file.
		f := inst.Files()[0]
		So(f.Name(), ShouldEqual, ".cipdpkg/manifest.json")
		So(f.Executable(), ShouldBeFalse)
		r, err := f.Open()
		if r != nil {
			defer r.Close()
		}
		So(err, ShouldBeNil)
		manifest, err := ioutil.ReadAll(r)
		So(err, ShouldBeNil)

		goodManifest := `{
			"format_version": "1.1",
			"package_name": "testing"
		}`
		So(string(manifest), shouldBeSameJSONDict, goodManifest)
	})

	Convey("Open empty package with unexpected instance ID", t, func() {
		// Build an empty package.
		out := bytes.Buffer{}
		err := BuildInstance(ctx, BuildInstanceOptions{
			Output:           &out,
			PackageName:      "testing",
			CompressionLevel: 5,
		})
		So(err, ShouldBeNil)

		// Attempt to open it, providing correct instance ID, should work.
		source := bytesFile(out.Bytes())
		inst, err := OpenInstance(ctx, source, "b1b76479c47e4ea4da7b6f4629c0d58ff7dc6569", VerifyHash)
		So(err, ShouldBeNil)
		So(inst, ShouldNotBeNil)
		So(inst.Pin(), ShouldResemble, Pin{"testing", "b1b76479c47e4ea4da7b6f4629c0d58ff7dc6569"})

		// Attempt to open it, providing incorrect instance ID.
		inst, err = OpenInstance(ctx, source, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", VerifyHash)
		So(err, ShouldNotBeNil)
		So(inst, ShouldBeNil)

		// Open with incorrect instance ID, but skipping the verification..
		inst, err = OpenInstance(ctx, source, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SkipHashVerification)
		So(err, ShouldBeNil)
		So(inst, ShouldNotBeNil)
		So(inst.Pin(), ShouldResemble, Pin{"testing", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	})

	Convey("OpenInstanceFile works", t, func() {
		// Open temp file.
		tempFile, err := ioutil.TempFile("", "cipdtest")
		So(err, ShouldBeNil)
		tempFilePath := tempFile.Name()
		defer os.Remove(tempFilePath)

		// Write empty package to it.
		err = BuildInstance(ctx, BuildInstanceOptions{
			Output:           tempFile,
			PackageName:      "testing",
			CompressionLevel: 5,
		})
		So(err, ShouldBeNil)
		tempFile.Close()

		// Read the package.
		inst, closer, err := OpenInstanceFile(ctx, tempFilePath, "", VerifyHash)
		So(err, ShouldBeNil)
		defer closer()
		So(inst, ShouldNotBeNil)
	})

	Convey("ExtractInstance works", t, func() {
		testMTime := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)

		inFiles := []File{
			NewTestFile("testing/qwerty", "12345", TestFileOpts{}),
			NewTestFile("abc", "duh", TestFileOpts{Executable: true}),
			NewTestFile("bad_dir/pkg/0/description.json", "{}", TestFileOpts{}),
			NewTestFile("writable", "write me", TestFileOpts{Writable: true}),
			NewTestFile("timestamped", "I'm old", TestFileOpts{ModTime: testMTime}),
			NewTestSymlink("rel_symlink", "abc"),
			NewTestSymlink("abs_symlink", "/abc/def"),
		}
		if runtime.GOOS == "windows" {
			inFiles = append(inFiles,
				NewWinTestFile("secret", "ninja", WinAttrHidden),
				NewWinTestFile("system", "machine", WinAttrSystem),
			)
		}

		out := bytes.Buffer{}
		err := BuildInstance(ctx, BuildInstanceOptions{
			Input:            inFiles,
			Output:           &out,
			PackageName:      "testing",
			VersionFile:      "subpath/version.json",
			CompressionLevel: 5,
		})
		So(err, ShouldBeNil)

		// Extract files.
		inst, err := OpenInstance(ctx, bytesFile(out.Bytes()), "", VerifyHash)
		So(err, ShouldBeNil)
		dest := &testDestination{}
		err = ExtractInstanceTxn(ctx, inst, dest, func(f File) bool {
			return strings.HasPrefix(f.Name(), "bad_dir/")
		})
		So(err, ShouldBeNil)
		So(dest.beginCalls, ShouldEqual, 1)
		So(dest.endCalls, ShouldEqual, 1)

		// Verify file list, file data and flags are correct.
		names := make([]string, len(dest.files))
		for i, f := range dest.files {
			names[i] = f.name
		}
		if runtime.GOOS != "windows" {
			So(names, ShouldResemble, []string{
				"testing/qwerty",
				"abc",
				"writable",
				"timestamped",
				"rel_symlink",
				"abs_symlink",
				"subpath/version.json",
				".cipdpkg/manifest.json",
			})
		} else {
			So(names, ShouldResemble, []string{
				"testing/qwerty",
				"abc",
				"writable",
				"timestamped",
				"rel_symlink",
				"abs_symlink",
				"secret",
				"system",
				"subpath/version.json",
				".cipdpkg/manifest.json",
			})
		}

		So(string(dest.fileByName("testing/qwerty").Bytes()), ShouldEqual, "12345")
		So(dest.fileByName("abc").executable, ShouldBeTrue)
		So(dest.fileByName("abc").writable, ShouldBeFalse)
		So(dest.fileByName("writable").writable, ShouldBeTrue)
		So(dest.fileByName("writable").modtime.IsZero(), ShouldBeTrue)
		So(dest.fileByName("timestamped").modtime, ShouldEqual, testMTime)
		So(dest.fileByName("rel_symlink").symlinkTarget, ShouldEqual, "abc")
		So(dest.fileByName("abs_symlink").symlinkTarget, ShouldEqual, "/abc/def")

		// Verify version file is correct.
		goodVersionFile := `{
			"instance_id": "284036fec1eaf6492a4e75d1db0920be5e8b3fd7",
			"package_name": "testing"
		}`
		if runtime.GOOS == "windows" {
			goodVersionFile = `{
				"instance_id": "7745210cceeb08f108cbb81122f5dbe3a415aaa2",
				"package_name": "testing"
			}`
		}
		So(string(dest.fileByName("subpath/version.json").Bytes()),
			shouldBeSameJSONDict, goodVersionFile)

		// Verify manifest file is correct.
		goodManifest := `{
			"format_version": "1.1",
			"package_name": "testing",
			"version_file": "subpath/version.json",
			"files": [
				{
					"name": "testing/qwerty",
					"size": 5
				},
				{
					"name": "abc",
					"size": 3,
					"executable": true
				},
				{
					"name": "writable",
					"size": 8,
					"writable": true
				},
				{
					"modtime":1514764800,
					"name": "timestamped",
					"size": 7
				},
				{
					"name": "rel_symlink",
					"size": 0,
					"symlink": "abc"
				},
				{
					"name": "abs_symlink",
					"size": 0,
					"symlink": "/abc/def"
				}%s,
				{
					"name": "subpath/version.json",
					"size": 92
				}
			]
		}`
		if runtime.GOOS == "windows" {
			goodManifest = fmt.Sprintf(goodManifest, `,{
				"name": "secret",
				"size": 5,
				"win_attrs": "H"
			},
			{
				"name": "system",
				"size": 7,
				"win_attrs": "S"
			}`)
		} else {
			goodManifest = fmt.Sprintf(goodManifest, "")
		}
		So(string(dest.fileByName(".cipdpkg/manifest.json").Bytes()),
			shouldBeSameJSONDict, goodManifest)
	})

	Convey("ExtractInstance handles v1 packages correctly", t, func() {
		// ZipInfos in packages with format_version "1" always have the writable bit
		// set, and always have 0 timestamp. During the extraction of such package,
		// the writable bit should be cleared, and the timestamp should not be reset
		// to 0 (it will be set to whatever the current time is).
		inFiles := []File{
			NewTestFile("testing/qwerty", "12345", TestFileOpts{Writable: true}),
			NewTestFile("abc", "duh", TestFileOpts{Executable: true, Writable: true}),
			NewTestFile("bad_dir/pkg/0/description.json", "{}", TestFileOpts{Writable: true}),
			NewTestSymlink("rel_symlink", "abc"),
			NewTestSymlink("abs_symlink", "/abc/def"),
		}
		if runtime.GOOS == "windows" {
			inFiles = append(inFiles,
				NewWinTestFile("secret", "ninja", WinAttrHidden),
				NewWinTestFile("system", "machine", WinAttrSystem),
			)
		}

		out := bytes.Buffer{}
		err := BuildInstance(ctx, BuildInstanceOptions{
			Input:                 inFiles,
			Output:                &out,
			PackageName:           "testing",
			VersionFile:           "subpath/version.json",
			CompressionLevel:      5,
			overrideFormatVersion: "1",
		})
		So(err, ShouldBeNil)

		// Extract files.
		inst, err := OpenInstance(ctx, bytesFile(out.Bytes()), "", VerifyHash)
		So(err, ShouldBeNil)
		dest := &testDestination{}
		err = ExtractInstanceTxn(ctx, inst, dest, func(f File) bool {
			return strings.HasPrefix(f.Name(), "bad_dir/")
		})
		So(err, ShouldBeNil)
		So(dest.beginCalls, ShouldEqual, 1)
		So(dest.endCalls, ShouldEqual, 1)

		// Verify file list, file data and flags are correct.
		names := make([]string, len(dest.files))
		for i, f := range dest.files {
			names[i] = f.name
		}
		if runtime.GOOS != "windows" {
			So(names, ShouldResemble, []string{
				"testing/qwerty",
				"abc",
				"rel_symlink",
				"abs_symlink",
				"subpath/version.json",
				".cipdpkg/manifest.json",
			})
		} else {
			So(names, ShouldResemble, []string{
				"testing/qwerty",
				"abc",
				"rel_symlink",
				"abs_symlink",
				"secret",
				"system",
				"subpath/version.json",
				".cipdpkg/manifest.json",
			})
		}

		So(string(dest.fileByName("testing/qwerty").Bytes()), ShouldEqual, "12345")
		So(dest.fileByName("abc").executable, ShouldBeTrue)
		So(dest.fileByName("abc").writable, ShouldBeFalse)
		So(dest.fileByName("rel_symlink").symlinkTarget, ShouldEqual, "abc")
		So(dest.fileByName("abs_symlink").symlinkTarget, ShouldEqual, "/abc/def")

		// Verify version file is correct.
		goodVersionFile := `{
			"instance_id": "2caf604d611300332f4f5d97dc2e3e1b40b9fc10",
			"package_name": "testing"
		}`
		if runtime.GOOS == "windows" {
			goodVersionFile = `{
				"instance_id": "64ba7831cc5374c8747e24315674b2b966065c5e",
				"package_name": "testing"
			}`
		}
		So(string(dest.fileByName("subpath/version.json").Bytes()),
			shouldBeSameJSONDict, goodVersionFile)

		// Verify manifest file is correct.
		goodManifest := `{
			"format_version": "1",
			"package_name": "testing",
			"version_file": "subpath/version.json",
			"files": [
				{
					"name": "testing/qwerty",
					"size": 5
				},
				{
					"name": "abc",
					"size": 3,
					"executable": true
				},
				{
					"name": "rel_symlink",
					"size": 0,
					"symlink": "abc"
				},
				{
					"name": "abs_symlink",
					"size": 0,
					"symlink": "/abc/def"
				}%s,
				{
					"name": "subpath/version.json",
					"size": 92
				}
			]
		}`
		if runtime.GOOS == "windows" {
			goodManifest = fmt.Sprintf(goodManifest, `,{
				"name": "secret",
				"size": 5,
				"win_attrs": "H"
			},
			{
				"name": "system",
				"size": 7,
				"win_attrs": "S"
			}`)
		} else {
			goodManifest = fmt.Sprintf(goodManifest, "")
		}
		So(string(dest.fileByName(".cipdpkg/manifest.json").Bytes()),
			shouldBeSameJSONDict, goodManifest)
	})
}

////////////////////////////////////////////////////////////////////////////////

type testDestination struct {
	beginCalls int
	endCalls   int
	files      []*testDestinationFile
}

type testDestinationFile struct {
	bytes.Buffer
	name          string
	executable    bool
	writable      bool
	modtime       time.Time
	symlinkTarget string
	winAttrs      WinAttrs
}

func (d *testDestinationFile) Close() error { return nil }

func (d *testDestination) Begin(context.Context) error {
	d.beginCalls++
	return nil
}

func (d *testDestination) CreateFile(ctx context.Context, name string, opts CreateFileOptions) (io.WriteCloser, error) {
	f := &testDestinationFile{
		name:       name,
		executable: opts.Executable,
		writable:   opts.Writable,
		modtime:    opts.ModTime,
		winAttrs:   opts.WinAttrs,
	}
	d.files = append(d.files, f)
	return f, nil
}

func (d *testDestination) CreateSymlink(ctx context.Context, name string, target string) error {
	f := &testDestinationFile{
		name:          name,
		symlinkTarget: target,
	}
	d.files = append(d.files, f)
	return nil
}

func (d *testDestination) End(ctx context.Context, success bool) error {
	d.endCalls++
	return nil
}

func (d *testDestination) fileByName(name string) *testDestinationFile {
	for _, f := range d.files {
		if f.name == name {
			return f
		}
	}
	return nil
}
