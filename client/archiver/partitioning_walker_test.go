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
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"go.chromium.org/luci/client/internal/common"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/system/filesystem"
	"go.chromium.org/luci/common/testing/testfs"
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

// fsView constructs a FilesystemView with no blacklist.
func fsView(root string) common.FilesystemView {
	fsView, err := common.NewFilesystemView(root, nil)
	if err != nil {
		// NewFilesystemView only fails due to bad blacklists. So this should never occur.
		panic("unexpected failure to construct FilesytemView")
	}
	return fsView
}

func TestWalkFn(t *testing.T) {
	testfs.MustWithTempDir(t, "", func(dir string) {

		type testCase struct {
			name      string
			files     []file
			walkFnErr error
			want      partitionedDeps
		}

		patha := filepath.Join(dir, "/rootDir/patha")
		pathb := filepath.Join(dir, "/rootDir/pathb")
		pathc := filepath.Join(dir, "/rootDir/pathc")

		pathInRegular := filepath.Join(dir, "rootDir", "inregular")
		pathOutRegular := filepath.Join(dir, "outregular")
		pathInSymlink := filepath.Join(dir, "rootDir", "insymlink")
		pathOutSymlink := filepath.Join(dir, "rootDir", "outsymlink")
		pathASymlink := filepath.Join(dir, "rootDir", "asymlink")

		rootDir := filepath.Join(dir, "rootDir")

		if err := filesystem.MakeDirs(rootDir); err != nil {
			t.Fatalf("failed to call filesystem.MakeDirs(%s): %v", rootDir, err)
		}

		fi, err := os.Create(pathInRegular)
		if err != nil {
			t.Fatalf("failed to call os.Create(%s): %v", pathInRegular, err)
		}
		if err := fi.Close(); err != nil {
			t.Fatalf("failed to call Close: %v", err)
		}

		fi, err = os.Create(pathOutRegular)
		if err != nil {
			t.Fatalf("failed to call os.Create(%s): %v", pathOutRegular, err)
		}
		if err := fi.Close(); err != nil {
			t.Fatalf("failed to call Close: %v", err)
		}

		statOutRegular, err := os.Stat(pathOutRegular)
		if err != nil {
			t.Fatalf("failed to call os.Stat(%s)", pathOutRegular)
		}

		if err := os.Symlink("inregular", pathInSymlink); err != nil {
			t.Fatalf("failed to call os.Symlink(%s, %s): %v", "inregular", pathInSymlink, err)
		}

		if err := os.Symlink("inregular", pathASymlink); err != nil {
			t.Fatalf("failed to call os.Symlink(%s, %s): %v", "inregular", pathASymlink, err)
		}

		if err := os.Symlink("../outregular", pathOutSymlink); err != nil {
			t.Fatalf("failed to call os.Symlink(%s, %s): %v", "inregular", pathInSymlink, err)
		}

		testCases := []testCase{
			{
				name: "partitions files",
				files: []file{
					symlink(pathInSymlink, 10e3),
					regularFile(pathb, 100e3),
					// The duplicate entry will be skipped.
					regularFile(pathb, 100e3),
					regularFile(pathc, 1000e3),
				},
				want: partitionedDeps{
					links: itemGroup{
						items: []*Item{
							{
								Path:    pathInSymlink,
								RelPath: "insymlink",
								Mode:    os.ModeSymlink,
								Size:    10e3,
							},
						},
						totalSize: 10e3,
					},
					filesToArchive: itemGroup{
						items: []*Item{
							{
								Path:    pathb,
								RelPath: "pathb",
								Mode:    0,
								Size:    100e3,
							},
						},
						totalSize: 100e3,
					},
					indivFiles: itemGroup{
						items: []*Item{
							{
								Path:    pathc,
								RelPath: "pathc",
								Mode:    0,
								Size:    1000e3,
							},
						},
						totalSize: 1000e3,
					},
				},
			},
			{
				name: "handles zero-size files",
				files: []file{
					symlink(pathInSymlink, 0),
					regularFile(pathb, 0),
				},
				want: partitionedDeps{
					links: itemGroup{
						items: []*Item{
							{
								Path:    pathInSymlink,
								RelPath: "insymlink",
								Mode:    os.ModeSymlink,
								Size:    0,
							},
						},
						totalSize: 0,
					},
					filesToArchive: itemGroup{
						items: []*Item{
							{
								Path:    pathb,
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
					symlink(pathASymlink, 1),
					symlink(pathInSymlink, 1<<1),
				},
				want: partitionedDeps{
					links: itemGroup{
						items: []*Item{
							{
								Path:    pathASymlink,
								RelPath: "asymlink",
								Mode:    os.ModeSymlink,
								Size:    1,
							},
							{
								Path:    pathInSymlink,
								RelPath: "insymlink",
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
					regularFile(patha, 1024<<10),
					regularFile(pathb, 1024<<11),
				},
				want: partitionedDeps{
					indivFiles: itemGroup{
						items: []*Item{
							{
								Path:    patha,
								RelPath: "patha",
								Mode:    0,
								Size:    1024 << 10,
							},
							{
								Path:    pathb,
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
					regularFile(patha, 1024),
					regularFile(pathb, 1024<<1),
				},
				want: partitionedDeps{
					filesToArchive: itemGroup{
						items: []*Item{
							{
								Path:    patha,
								RelPath: "patha",
								Mode:    0,
								Size:    1024,
							},
							{
								Path:    pathb,
								RelPath: "pathb",
								Mode:    0,
								Size:    1024 << 1,
							},
						},
						totalSize: 1024 + 2048,
					},
				},
			},
			{
				name: "out of tree symlink",
				files: []file{
					symlink(pathOutSymlink, 0),
				},
				want: partitionedDeps{
					filesToArchive: itemGroup{
						items: []*Item{
							{
								Path:    pathOutRegular,
								RelPath: "outsymlink",
								Mode:    statOutRegular.Mode(),
								Size:    0,
							},
						},
						totalSize: 0,
					},
				},
			},
		}

	TestCases:
		for _, tc := range testCases {
			pw := partitioningWalker{fsView: fsView(filepath.Join(dir, "/rootDir")), seen: stringset.New(0)}
			for _, f := range tc.files {
				if err := pw.walkFn(f.path, f.bfi, tc.walkFnErr); err != nil {
					t.Errorf("partitioning deps(%s): walkFn got err %v; want nil", tc.name, err)
					continue TestCases
				}
			}

			if got, want := pw.parts, tc.want; !reflect.DeepEqual(got, want) {
				t.Errorf("partitioning deps(%s):\ngot\n%#v\nwant\n%#v", tc.name, got, want)
			}
		}
	})()
}

func TestWalkFn_BadRelpath(t *testing.T) {
	pw := partitioningWalker{fsView: fsView("/rootDir"), seen: stringset.New(0)}
	f := regularFile("./patha", 1)
	if err := pw.walkFn(f.path, f.bfi, nil); err == nil {
		t.Errorf("testing bad relpath: walkFn returned nil err; want non-nil err")
	}
}

func TestWalkFn_ReturnsErrorsUnchanged(t *testing.T) {
	pw := partitioningWalker{fsView: fsView("/rootDir"), seen: stringset.New(0)}
	f := regularFile("/rootDir/patha", 1)
	if err := pw.walkFn(f.path, f.bfi, errBang); err != errBang {
		t.Errorf("walkFn err: got: %v; want %v", err, errBang)
	}
}

func TestWalkFn_DoesNothingForDirectories(t *testing.T) {
	pw := partitioningWalker{fsView: fsView("/rootDir"), seen: stringset.New(0)}
	f := directory("/rootDir/patha")
	if err := pw.walkFn(f.path, f.bfi, nil); err != nil {
		t.Errorf("walkFn err: got: %v; want nil", err)
	}
}
