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
	"errors"
	"os"
	"reflect"
	"testing"
)

// basicFileInfo implements some of os.FileInfo, and panics if unexpected parts
// of that interface are called.
type basicFileInfo struct {
	size  int64
	mode  os.FileMode
	isDir bool

	os.FileInfo
}

func (bfi basicFileInfo) Size() int64       { return bfi.size }
func (bfi basicFileInfo) Mode() os.FileMode { return bfi.mode }
func (bfi basicFileInfo) IsDir() bool       { return bfi.isDir }

type file struct {
	path string
	bfi  basicFileInfo
}

func symlink(path string, size int64) file {
	return file{
		path: path,
		bfi: basicFileInfo{
			size:  size,
			mode:  os.ModeSymlink,
			isDir: false,
		},
	}
}

func regularFile(path string, size int64) file {
	return file{
		path: path,
		bfi: basicFileInfo{
			size:  size,
			mode:  0,
			isDir: false,
		},
	}
}

func directory(path string) file {
	return file{
		path: path,
		bfi: basicFileInfo{
			size:  0,
			mode:  os.ModeDir,
			isDir: true,
		},
	}
}

var errBang = errors.New("bang")

func TestWalkFn(t *testing.T) {
	type testCase struct {
		name      string
		files     []file
		walkFnErr error
		want      partitionedDeps
	}

	testCases := []testCase{
		{
			name: "partitions files",
			files: []file{
				symlink("/rootDir/patha", 1e3),
				regularFile("/rootDir/pathb", 10e3),
				regularFile("/rootDir/pathc", 100e3),
			},
			want: partitionedDeps{
				links: itemGroup{
					items: []*Item{
						{
							Path:    "/rootDir/patha",
							RelPath: "patha",
							Mode:    os.ModeSymlink,
							Size:    1e3,
						},
					},
					totalSize: 1e3,
				},
				filesToArchive: itemGroup{
					items: []*Item{
						{
							Path:    "/rootDir/pathb",
							RelPath: "pathb",
							Mode:    0,
							Size:    10e3,
						},
					},
					totalSize: 10e3,
				},
				indivFiles: itemGroup{
					items: []*Item{
						{
							Path:    "/rootDir/pathc",
							RelPath: "pathc",
							Mode:    0,
							Size:    100e3,
						},
					},
					totalSize: 100e3,
				},
			},
		},
		{
			name: "handles zero-size files",
			files: []file{
				symlink("/rootDir/patha", 0),
				regularFile("/rootDir/pathb", 0),
			},
			want: partitionedDeps{
				links: itemGroup{
					items: []*Item{
						{
							Path:    "/rootDir/patha",
							RelPath: "patha",
							Mode:    os.ModeSymlink,
							Size:    0,
						},
					},
					totalSize: 0,
				},
				filesToArchive: itemGroup{
					items: []*Item{
						{
							Path:    "/rootDir/pathb",
							RelPath: "pathb",
							Mode:    0,
							Size:    0,
						},
					},
					totalSize: 0,
				},
			},
		},
		{
			name: "aggregates link sizes",
			files: []file{
				symlink("/rootDir/patha", 1),
				symlink("/rootDir/pathb", 1<<1),
			},
			want: partitionedDeps{
				links: itemGroup{
					items: []*Item{
						{
							Path:    "/rootDir/patha",
							RelPath: "patha",
							Mode:    os.ModeSymlink,
							Size:    1,
						},
						{
							Path:    "/rootDir/pathb",
							RelPath: "pathb",
							Mode:    os.ModeSymlink,
							Size:    1 << 1,
						},
					},
					totalSize: 3,
				},
			},
		},
		{
			name: "aggregates large file sizes",
			files: []file{
				regularFile("/rootDir/patha", 1024<<10),
				regularFile("/rootDir/pathb", 1024<<11),
			},
			want: partitionedDeps{
				indivFiles: itemGroup{
					items: []*Item{
						{
							Path:    "/rootDir/patha",
							RelPath: "patha",
							Mode:    0,
							Size:    1024 << 10,
						},
						{
							Path:    "/rootDir/pathb",
							RelPath: "pathb",
							Mode:    0,
							Size:    1024 << 11,
						},
					},
					totalSize: (1024 << 10) + (1024 << 11),
				},
			},
		},
		{
			name: "aggregates small file sizes",
			files: []file{
				regularFile("/rootDir/patha", 1024),
				regularFile("/rootDir/pathb", 1024<<1),
			},
			want: partitionedDeps{
				filesToArchive: itemGroup{
					items: []*Item{
						{
							Path:    "/rootDir/patha",
							RelPath: "patha",
							Mode:    0,
							Size:    1024,
						},
						{
							Path:    "/rootDir/pathb",
							RelPath: "pathb",
							Mode:    0,
							Size:    1024 << 1,
						},
					},
					totalSize: 1024 + 2048,
				},
			},
		},
	}

TestCases:
	for _, tc := range testCases {
		pw := partitioningWalker{rootDir: "/rootDir"}
		for _, f := range tc.files {
			if err := pw.walkFn(f.path, f.bfi, tc.walkFnErr); err != nil {
				t.Errorf("partitioning deps(%s): walkFn got err %v; want nil", tc.name, err)
				continue TestCases
			}
		}
		if got, want := pw.parts, tc.want; !reflect.DeepEqual(got, want) {
			t.Errorf("partitioning deps(%s): got %#v; want %#v", tc.name, got, want)
		}
	}

}

func TestWalkFn_BadRelpath(t *testing.T) {
	pw := partitioningWalker{rootDir: "/rootDir"}
	f := regularFile("./patha", 1)
	if err := pw.walkFn(f.path, f.bfi, nil); err == nil {
		t.Errorf("testing bad relpath: walkFn returned nil err; want non-nil err")
	}
}

func TestWalkFn_ReturnsErrorsUnchanged(t *testing.T) {
	pw := partitioningWalker{rootDir: "/rootDir"}
	f := regularFile("/rootDir/patha", 1)
	if err := pw.walkFn(f.path, f.bfi, errBang); err != errBang {
		t.Errorf("walkFn err: got: %v; want %v", err, errBang)
	}
}

func TestWalkFn_DoesNothingForDirectories(t *testing.T) {
	pw := partitioningWalker{rootDir: "/rootDir"}
	f := directory("/rootDir/patha")
	if err := pw.walkFn(f.path, f.bfi, nil); err != nil {
		t.Errorf("walkFn err: got: %v; want nil", err)
	}
}
