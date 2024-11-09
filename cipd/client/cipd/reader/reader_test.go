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
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
	"go.chromium.org/luci/common/testing/truth/should"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/client/cipd/builder"
	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/pkg"

	. "go.chromium.org/luci/cipd/common"
)

func stringCounts(values []string) map[string]int {
	counts := make(map[string]int)
	for _, v := range values {
		counts[v]++
	}
	return counts
}

// shouldContainSameStrings checks if the left and right side are slices that
// contain the same strings, regardless of the ordering.
func shouldContainSameStrings(expected []string) comparison.Func[[]string] {
	return func(actual []string) *failure.Summary {
		return should.Match(stringCounts(expected))(stringCounts(actual))
	}
}

func normalizeJSON(s string) (string, error) {
	// Round trip through default json marshaller to normalize indentation.
	var x map[string]any
	if err := json.Unmarshal([]byte(s), &x); err != nil {
		return "", err
	}
	blob, err := json.Marshal(x)
	if err != nil {
		return "", err
	}
	return string(blob), nil
}

func shouldBeSameJSONDict(expected string) comparison.Func[string] {
	expectedNorm, err := normalizeJSON(expected)
	if err != nil {
		return func(_ string) *failure.Summary {
			return should.ErrLike(nil)(err)
		}
	}

	return func(actual string) *failure.Summary {
		actualNorm, err := normalizeJSON(actual)
		if err != nil {
			return should.ErrLike(nil)(err)
		}

		return should.Equal(expectedNorm)(actualNorm)
	}
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

	ftt.Run("Open empty package works", t, func(t *ftt.Test) {
		// Build an empty package.
		out := bytes.Buffer{}
		pin, err := builder.BuildInstance(ctx, builder.Options{
			Output:           &out,
			PackageName:      "testing",
			CompressionLevel: 5,
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pin, should.Resemble(Pin{
			PackageName: "testing",
			InstanceID:  "PPM180-5i-V1q5554ewKGO4jq4cWB-cOwTuyhoCv3joC",
		}))

		// Open it.
		inst, err := OpenInstance(ctx, bytesFile(&out), OpenInstanceOpts{
			VerificationMode: CalculateHash,
			HashAlgo:         api.HashAlgo_SHA256,
		})
		assert.Loosely(t, inst, should.NotBeNil)
		assert.Loosely(t, err, should.BeNil)
		defer inst.Close(ctx, false)

		assert.Loosely(t, inst.Pin(), should.Resemble(pin))
		assert.Loosely(t, len(inst.Files()), should.Equal(1))

		// CalculatePin also agrees with the value of the pin.
		calcedPin, err := CalculatePin(ctx, pkg.NewBytesSource(out.Bytes()), api.HashAlgo_SHA256)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, calcedPin, should.Resemble(pin))

		// Contains single manifest file.
		f := inst.Files()[0]
		assert.Loosely(t, f.Name(), should.Equal(".cipdpkg/manifest.json"))
		assert.Loosely(t, f.Executable(), should.BeFalse)
		r, err := f.Open()
		if r != nil {
			defer r.Close()
		}
		assert.Loosely(t, err, should.BeNil)
		manifest, err := io.ReadAll(r)
		assert.Loosely(t, err, should.BeNil)

		goodManifest := `{
			"format_version": "1.1",
			"package_name": "testing"
		}`
		assert.Loosely(t, string(manifest), shouldBeSameJSONDict(goodManifest))
	})

	ftt.Run("Open empty package with unexpected instance ID", t, func(t *ftt.Test) {
		// Build an empty package.
		out := bytes.Buffer{}
		_, err := builder.BuildInstance(ctx, builder.Options{
			Output:           &out,
			PackageName:      "testing",
			CompressionLevel: 5,
		})
		assert.Loosely(t, err, should.BeNil)

		// Attempt to open it, providing correct instance ID, should work.
		inst, err := OpenInstance(ctx, bytesFile(&out), OpenInstanceOpts{
			VerificationMode: VerifyHash,
			InstanceID:       "PPM180-5i-V1q5554ewKGO4jq4cWB-cOwTuyhoCv3joC",
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, inst, should.NotBeNil)
		defer inst.Close(ctx, false)

		assert.Loosely(t, inst.Pin(), should.Resemble(Pin{
			PackageName: "testing",
			InstanceID:  "PPM180-5i-V1q5554ewKGO4jq4cWB-cOwTuyhoCv3joC",
		}))

		// Attempt to open it, providing incorrect instance ID.
		inst, err = OpenInstance(ctx, bytesFile(&out), OpenInstanceOpts{
			VerificationMode: VerifyHash,
			InstanceID:       "ZZZZZZZZ_LlIHZUsZlTzpmiCs8AqvAhz9TZzN96Qpx4C",
		})
		assert.Loosely(t, err, should.Equal(ErrHashMismatch))
		assert.Loosely(t, inst, should.BeNil)

		// Open with incorrect instance ID, but skipping the verification..
		inst, err = OpenInstance(ctx, bytesFile(&out), OpenInstanceOpts{
			VerificationMode: SkipHashVerification,
			InstanceID:       "ZZZZZZZZ_LlIHZUsZlTzpmiCs8AqvAhz9TZzN96Qpx4C",
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, inst, should.NotBeNil)
		defer inst.Close(ctx, false)

		assert.Loosely(t, inst.Pin(), should.Resemble(Pin{
			PackageName: "testing",
			InstanceID:  "ZZZZZZZZ_LlIHZUsZlTzpmiCs8AqvAhz9TZzN96Qpx4C",
		}))
	})

	ftt.Run("OpenInstanceFile works", t, func(t *ftt.Test) {
		// Open temp file.
		tempFile, err := ioutil.TempFile("", "cipdtest")
		assert.Loosely(t, err, should.BeNil)
		tempFilePath := tempFile.Name()
		defer os.Remove(tempFilePath)

		// Write empty package to it.
		_, err = builder.BuildInstance(ctx, builder.Options{
			Output:           tempFile,
			PackageName:      "testing",
			CompressionLevel: 5,
		})
		assert.Loosely(t, err, should.BeNil)
		tempFile.Close()

		// Read the package.
		inst, err := OpenInstanceFile(ctx, tempFilePath, OpenInstanceOpts{
			VerificationMode: CalculateHash,
			HashAlgo:         api.HashAlgo_SHA256,
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, inst, should.NotBeNil)
		assert.Loosely(t, inst.Close(ctx, false), should.BeNil)
	})

	ftt.Run("ExtractFiles works", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)

		// Extract files.
		inst, err := OpenInstance(ctx, bytesFile(&out), OpenInstanceOpts{
			VerificationMode: CalculateHash,
			HashAlgo:         api.HashAlgo_SHA256,
		})
		assert.Loosely(t, err, should.BeNil)
		defer inst.Close(ctx, false)

		dest := &testDestination{}
		_, err = ExtractFilesTxn(ctx, inst.Files(), dest, 16, pkg.WithManifest, "")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, dest.beginCalls, should.Equal(1))
		assert.Loosely(t, dest.endCalls, should.Equal(1))

		// Verify file list, file data and flags are correct.
		names := make([]string, len(dest.files))
		for i, f := range dest.files {
			names[i] = f.name
		}
		if runtime.GOOS != "windows" {
			assert.Loosely(t, names, shouldContainSameStrings([]string{
				"testing/qwerty",
				"abc",
				"writable",
				"timestamped",
				"rel_symlink",
				"abs_symlink",
				"subpath/version.json",
				".cipdpkg/manifest.json",
			}))
		} else {
			assert.Loosely(t, names, shouldContainSameStrings([]string{
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
			}))
		}

		assert.Loosely(t, string(dest.fileByName("testing/qwerty").Bytes()), should.Equal("12345"))
		assert.Loosely(t, dest.fileByName("abc").executable, should.BeTrue)
		assert.Loosely(t, dest.fileByName("abc").writable, should.BeFalse)
		assert.Loosely(t, dest.fileByName("writable").writable, should.BeTrue)
		assert.Loosely(t, dest.fileByName("writable").modtime.IsZero(), should.BeTrue)
		assert.Loosely(t, dest.fileByName("timestamped").modtime, should.Match(testMTime))
		assert.Loosely(t, dest.fileByName("rel_symlink").symlinkTarget, should.Equal("abc"))
		assert.Loosely(t, dest.fileByName("abs_symlink").symlinkTarget, should.Equal("/abc/def"))

		// Verify version file is correct.
		goodVersionFile := `{
			"instance_id": "OvNF-MsVw1eXYJtjkiq7pXCm6mLYYSaN9qSqsMT3DEAC",
			"package_name": "testing"
		}`
		if runtime.GOOS == "windows" {
			goodVersionFile = `{
				"instance_id": "ZM2WksvyI1lQiHnYcuLLXQSNquBPexuH-t57CkJPVDoC",
				"package_name": "testing"
			}`
		}
		assert.Loosely(t, string(dest.fileByName("subpath/version.json").Bytes()),
			shouldBeSameJSONDict(goodVersionFile))

		// Verify manifest file is correct.
		goodManifest := `{
			"format_version": "1.1",
			"package_name": "testing",
			"version_file": "subpath/version.json",
			"files": [
				{
					"name": "testing/qwerty",
					"size": 5,
					"hash": "WZRHGrsBESr8wYFZ9sx0tPURuZgG2lmzyvWpwXPKz8UC"
				},
				{
					"name": "abc",
					"size": 3,
					"executable": true,
					"hash": "i_jQPvLtCYwT3iPForJuG9tFWRu9c3ndgjxk7nXjY2kC"
				},
				{
					"name": "writable",
					"size": 8,
					"writable": true,
					"hash": "QeUFaPVoXLyp7lPHwnWGgBD5Wo-buja_bBGTx4s3jkkC"
				},
				{
					"modtime":1514764800,
					"name": "timestamped",
					"size": 7,
					"hash": "M2fO8ZiWvyqNVmp_Nu5QZo80JXSjkqHz60zRlhqNHzgC"
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
				},%s
			]
		}`
		if runtime.GOOS == "windows" {
			goodManifest = fmt.Sprintf(goodManifest, `{
				"name": "secret",
				"size": 5,
				"win_attrs": "H",
				"hash": "VEgllRdxFuYQOwdtvzBkjl0FN90e2c9a5FYvqKcA1HsC"
			},
			{
				"name": "system",
				"size": 7,
				"win_attrs": "S",
				"hash": "vAIKNbf5yxOC57U0xo48Ux2EmxGb8U913erWzEXDzMEC"
			},
			{
				"name": "subpath/version.json",
				"size": 96,
				"hash": "LQ25MuK852zm7GjAUcBIzWpAzhcXV7L4pEVl2LCUz8YC"
			}`)
		} else {
			goodManifest = fmt.Sprintf(goodManifest, `{
				"name": "subpath/version.json",
				"size": 96,
				"hash": "XD6QlRyLX4Cj09wtPLAEQGacygrySk317U38Ku2d9zIC"
			}`)
		}
		assert.Loosely(t, string(dest.fileByName(".cipdpkg/manifest.json").Bytes()),
			shouldBeSameJSONDict(goodManifest))
	})

	ftt.Run("ExtractFiles handles v1 packages correctly", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)

		// Extract files.
		inst, err := OpenInstance(ctx, bytesFile(&out), OpenInstanceOpts{
			VerificationMode: CalculateHash,
			HashAlgo:         api.HashAlgo_SHA256,
		})
		assert.Loosely(t, err, should.BeNil)
		defer inst.Close(ctx, false)

		dest := &testDestination{}
		_, err = ExtractFilesTxn(ctx, inst.Files(), dest, 16, pkg.WithManifest, "")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, dest.beginCalls, should.Equal(1))
		assert.Loosely(t, dest.endCalls, should.Equal(1))

		// Verify file list, file data and flags are correct.
		names := make([]string, len(dest.files))
		for i, f := range dest.files {
			names[i] = f.name
		}
		if runtime.GOOS != "windows" {
			assert.Loosely(t, names, shouldContainSameStrings([]string{
				"testing/qwerty",
				"abc",
				"rel_symlink",
				"abs_symlink",
				"subpath/version.json",
				".cipdpkg/manifest.json",
			}))
		} else {
			assert.Loosely(t, names, shouldContainSameStrings([]string{
				"testing/qwerty",
				"abc",
				"rel_symlink",
				"abs_symlink",
				"secret",
				"system",
				"subpath/version.json",
				".cipdpkg/manifest.json",
			}))
		}

		assert.Loosely(t, string(dest.fileByName("testing/qwerty").Bytes()), should.Equal("12345"))
		assert.Loosely(t, dest.fileByName("abc").executable, should.BeTrue)
		assert.Loosely(t, dest.fileByName("abc").writable, should.BeFalse)
		assert.Loosely(t, dest.fileByName("rel_symlink").symlinkTarget, should.Equal("abc"))
		assert.Loosely(t, dest.fileByName("abs_symlink").symlinkTarget, should.Equal("/abc/def"))

		// Verify version file is correct.
		goodVersionFile := `{
			"instance_id": "TFCuWXMWQSAoTRwjHXIu9ZTRTNrkwxuXXNKg_HTFQn0C",
			"package_name": "testing"
		}`
		if runtime.GOOS == "windows" {
			goodVersionFile = `{
				"instance_id": "JtKHZQLjvjsDbo2bRjfkCXCdlE4PZ-mRA7c2fAx09hkC",
				"package_name": "testing"
			}`
		}
		assert.Loosely(t, string(dest.fileByName("subpath/version.json").Bytes()),
			shouldBeSameJSONDict(goodVersionFile))

		// Verify manifest file is correct.
		goodManifest := `{
			"format_version": "1",
			"package_name": "testing",
			"version_file": "subpath/version.json",
			"files": [
				{
					"name": "testing/qwerty",
					"size": 5,
					"hash": "WZRHGrsBESr8wYFZ9sx0tPURuZgG2lmzyvWpwXPKz8UC"
				},
				{
					"name": "abc",
					"size": 3,
					"executable": true,
					"hash": "i_jQPvLtCYwT3iPForJuG9tFWRu9c3ndgjxk7nXjY2kC"
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
				},%s
			]
		}`
		if runtime.GOOS == "windows" {
			goodManifest = fmt.Sprintf(goodManifest, `{
				"name": "secret",
				"size": 5,
				"win_attrs": "H",
				"hash": "VEgllRdxFuYQOwdtvzBkjl0FN90e2c9a5FYvqKcA1HsC"
			},
			{
				"name": "system",
				"size": 7,
				"win_attrs": "S",
				"hash": "vAIKNbf5yxOC57U0xo48Ux2EmxGb8U913erWzEXDzMEC"
			},
			{
				"name": "subpath/version.json",
				"size": 96,
				"hash": "NktdGybrepm6oPSqjFG_qrNZDeaY_KbP4elWDz4ww48C"
			}`)
		} else {
			goodManifest = fmt.Sprintf(goodManifest, `{
				"name": "subpath/version.json",
				"size": 96,
				"hash": "JlgQS4Xa4D7f94PYzpQcvgPsDfQqySYVlUBBYfF6x8sC"
			}`)
		}
		assert.Loosely(t, string(dest.fileByName(".cipdpkg/manifest.json").Bytes()),
			shouldBeSameJSONDict(goodManifest))
	})
}

////////////////////////////////////////////////////////////////////////////////

type testDestination struct {
	beginCalls       int
	endCalls         int
	files            []*testDestinationFile
	registerFileLock sync.Mutex
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
	d.registerFile(f)
	return f, nil
}

func (d *testDestination) CreateSymlink(ctx context.Context, name string, target string) error {
	f := &testDestinationFile{
		name:          name,
		symlinkTarget: target,
	}
	d.registerFile(f)
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

func (d *testDestination) registerFile(f *testDestinationFile) {
	d.registerFileLock.Lock()
	d.files = append(d.files, f)
	d.registerFileLock.Unlock()
}
