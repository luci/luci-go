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

package reader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"runtime"
	"testing"
	"time"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/client/cipd/builder"
	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/pkg"

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

type bytesSource struct {
	*bytes.Reader
}

func (bytesSource) Close(context.Context, bool) error { return nil }

func bytesFile(buf *bytes.Buffer) pkg.Source {
	return bytesSource{bytes.NewReader(buf.Bytes())}
}

func TestPackageReading(t *testing.T) {
	ctx := context.Background()

	Convey("Open empty package works", t, func() {
		// Build an empty package.
		out := bytes.Buffer{}
		pin, err := builder.BuildInstance(ctx, builder.Options{
			Output:           &out,
			PackageName:      "testing",
			CompressionLevel: 5,
		})
		So(err, ShouldBeNil)
		So(pin, ShouldResemble, Pin{
			PackageName: "testing",
			InstanceID:  "ZrRKa9HN_LlIHZUsZlTzpmiCs8AqvAhz9TZzN96Qpx4C",
		})

		// Open it.
		inst, err := OpenInstance(ctx, bytesFile(&out), OpenInstanceOpts{
			VerificationMode: CalculateHash,
			HashAlgo:         api.HashAlgo_SHA256,
		})
		So(inst, ShouldNotBeNil)
		So(err, ShouldBeNil)
		defer inst.Close(ctx, false)

		So(inst.Pin(), ShouldResemble, pin)
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
		_, err := builder.BuildInstance(ctx, builder.Options{
			Output:           &out,
			PackageName:      "testing",
			CompressionLevel: 5,
		})
		So(err, ShouldBeNil)

		// Attempt to open it, providing correct instance ID, should work.
		inst, err := OpenInstance(ctx, bytesFile(&out), OpenInstanceOpts{
			VerificationMode: VerifyHash,
			InstanceID:       "ZrRKa9HN_LlIHZUsZlTzpmiCs8AqvAhz9TZzN96Qpx4C",
		})
		So(err, ShouldBeNil)
		So(inst, ShouldNotBeNil)
		defer inst.Close(ctx, false)

		So(inst.Pin(), ShouldResemble, Pin{
			PackageName: "testing",
			InstanceID:  "ZrRKa9HN_LlIHZUsZlTzpmiCs8AqvAhz9TZzN96Qpx4C",
		})

		// Attempt to open it, providing incorrect instance ID.
		inst, err = OpenInstance(ctx, bytesFile(&out), OpenInstanceOpts{
			VerificationMode: VerifyHash,
			InstanceID:       "ZZZZZZZZ_LlIHZUsZlTzpmiCs8AqvAhz9TZzN96Qpx4C",
		})
		So(err, ShouldEqual, ErrHashMismatch)
		So(inst, ShouldBeNil)

		// Open with incorrect instance ID, but skipping the verification..
		inst, err = OpenInstance(ctx, bytesFile(&out), OpenInstanceOpts{
			VerificationMode: SkipHashVerification,
			InstanceID:       "ZZZZZZZZ_LlIHZUsZlTzpmiCs8AqvAhz9TZzN96Qpx4C",
		})
		So(err, ShouldBeNil)
		So(inst, ShouldNotBeNil)
		defer inst.Close(ctx, false)

		So(inst.Pin(), ShouldResemble, Pin{
			PackageName: "testing",
			InstanceID:  "ZZZZZZZZ_LlIHZUsZlTzpmiCs8AqvAhz9TZzN96Qpx4C",
		})
	})

	Convey("OpenInstanceFile works", t, func() {
		// Open temp file.
		tempFile, err := ioutil.TempFile("", "cipdtest")
		So(err, ShouldBeNil)
		tempFilePath := tempFile.Name()
		defer os.Remove(tempFilePath)

		// Write empty package to it.
		_, err = builder.BuildInstance(ctx, builder.Options{
			Output:           tempFile,
			PackageName:      "testing",
			CompressionLevel: 5,
		})
		So(err, ShouldBeNil)
		tempFile.Close()

		// Read the package.
		inst, err := OpenInstanceFile(ctx, tempFilePath, OpenInstanceOpts{
			VerificationMode: CalculateHash,
			HashAlgo:         api.HashAlgo_SHA256,
		})
		So(err, ShouldBeNil)
		So(inst, ShouldNotBeNil)
		So(inst.Close(ctx, false), ShouldBeNil)
	})

	Convey("ExtractFiles works", t, func() {
		testMTime := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)

		inFiles := []fs.File{
			fs.NewTestFile("testing/qwerty", "12345", fs.TestFileOpts{}),
			fs.NewTestFile("abc", "duh", fs.TestFileOpts{Executable: true}),
			fs.NewTestFile("writable", "write me", fs.TestFileOpts{Writable: true}),
			fs.NewTestFile("timestamped", "I'm old", fs.TestFileOpts{ModTime: testMTime}),
			fs.NewTestSymlink("rel_symlink", "abc"),
			fs.NewTestSymlink("abs_symlink", "/abc/def"),
		}
		if runtime.GOOS == "windows" {
			inFiles = append(inFiles,
				fs.NewWinTestFile("secret", "ninja", fs.WinAttrHidden),
				fs.NewWinTestFile("system", "machine", fs.WinAttrSystem),
			)
		}

		out := bytes.Buffer{}
		_, err := builder.BuildInstance(ctx, builder.Options{
			Input:            inFiles,
			Output:           &out,
			PackageName:      "testing",
			VersionFile:      "subpath/version.json",
			CompressionLevel: 5,
		})
		So(err, ShouldBeNil)

		// Extract files.
		inst, err := OpenInstance(ctx, bytesFile(&out), OpenInstanceOpts{
			VerificationMode: CalculateHash,
			HashAlgo:         api.HashAlgo_SHA256,
		})
		So(err, ShouldBeNil)
		defer inst.Close(ctx, false)

		dest := &testDestination{}
		err = ExtractFilesTxn(ctx, inst.Files(), dest, pkg.WithManifest)
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
			"instance_id": "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
			"package_name": "testing"
		}`
		if runtime.GOOS == "windows" {
			goodVersionFile = `{
				"instance_id": "8YWDtsb0eJ3iegl5hBGaGJnw3qB2eNXhs9Fz7WO50B8C",
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
					"size": 96
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

	Convey("ExtractFiles handles v1 packages correctly", t, func() {
		// ZipInfos in packages with format_version "1" always have the writable bit
		// set, and always have 0 timestamp. During the extraction of such package,
		// the writable bit should be cleared, and the timestamp should not be reset
		// to 0 (it will be set to whatever the current time is).
		inFiles := []fs.File{
			fs.NewTestFile("testing/qwerty", "12345", fs.TestFileOpts{Writable: true}),
			fs.NewTestFile("abc", "duh", fs.TestFileOpts{Executable: true, Writable: true}),
			fs.NewTestSymlink("rel_symlink", "abc"),
			fs.NewTestSymlink("abs_symlink", "/abc/def"),
		}
		if runtime.GOOS == "windows" {
			inFiles = append(inFiles,
				fs.NewWinTestFile("secret", "ninja", fs.WinAttrHidden),
				fs.NewWinTestFile("system", "machine", fs.WinAttrSystem),
			)
		}

		out := bytes.Buffer{}
		_, err := builder.BuildInstance(ctx, builder.Options{
			Input:                 inFiles,
			Output:                &out,
			PackageName:           "testing",
			VersionFile:           "subpath/version.json",
			CompressionLevel:      5,
			OverrideFormatVersion: "1",
		})
		So(err, ShouldBeNil)

		// Extract files.
		inst, err := OpenInstance(ctx, bytesFile(&out), OpenInstanceOpts{
			VerificationMode: CalculateHash,
			HashAlgo:         api.HashAlgo_SHA256,
		})
		So(err, ShouldBeNil)
		defer inst.Close(ctx, false)

		dest := &testDestination{}
		err = ExtractFilesTxn(ctx, inst.Files(), dest, pkg.WithManifest)
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
			"instance_id": "Fmfp6kX5NlbedUcu_lw6ONfNaF_mKRb2_ZX78l3QJqcC",
			"package_name": "testing"
		}`
		if runtime.GOOS == "windows" {
			goodVersionFile = `{
				"instance_id": "oVEoO_i5iVpgGhvjxQL-D1Lp_Tn8LUQ-hBbh48651mYC",
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
					"size": 96
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
	winAttrs      fs.WinAttrs
}

func (d *testDestinationFile) Close() error { return nil }

func (d *testDestination) Begin(context.Context) error {
	d.beginCalls++
	return nil
}

func (d *testDestination) CreateFile(ctx context.Context, name string, opts fs.CreateFileOptions) (io.WriteCloser, error) {
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
