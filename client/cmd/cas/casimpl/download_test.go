// Copyright 2026 The LUCI Authors.
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

package casimpl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/fakes"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestPathValidation(t *testing.T) {
	t.Parallel()

	ftt.Run(`checkContained and validateOutputPath`, t, func(t *ftt.Test) {
		root := filepath.Join(string(filepath.Separator), "tmp", "download_root")

		t.Run(`Valid relative paths`, func(t *ftt.Test) {
			assert.NoErr(t, checkContained(root, filepath.Join(root, "file.txt")))
			assert.NoErr(t, checkContained(root, filepath.Join(root, "a", "b", "c.txt")))
			assert.NoErr(t, validateOutputPath(root, "file.txt", &client.TreeOutput{}))
			assert.NoErr(t, validateOutputPath(root, "a/b/c.txt", &client.TreeOutput{}))
			assert.NoErr(t, validateOutputPath(root, "a/../b/c.txt", &client.TreeOutput{}))
			if runtime.GOOS == "windows" {
				assert.NoErr(t, validateOutputPath(root, "a\\b\\c.txt", &client.TreeOutput{}))
				assert.NoErr(t, validateOutputPath(root, "a\\..\\b\\c.txt", &client.TreeOutput{}))
			}
		})

		t.Run(`Empty directory at root`, func(t *ftt.Test) {
			assert.NoErr(t, validateOutputPath(root, "", &client.TreeOutput{IsEmptyDirectory: true}))
			assert.NoErr(t, validateOutputPath(root, ".", &client.TreeOutput{IsEmptyDirectory: true}))
		})

		t.Run(`File resolving to root is rejected`, func(t *ftt.Test) {
			assert.ErrIsLike(t, validateOutputPath(root, "", &client.TreeOutput{IsEmptyDirectory: false}), "resolves to destination root")
			assert.ErrIsLike(t, validateOutputPath(root, ".", &client.TreeOutput{IsEmptyDirectory: false}), "resolves to destination root")
		})

		t.Run(`Traversal paths rejected`, func(t *ftt.Test) {
			assert.ErrIsLike(t, checkContained(root, filepath.Join(root, "..")), "escapes root")
			assert.ErrIsLike(t, checkContained(root, filepath.Join(root, "..", "escaped")), "escapes root")
			assert.ErrIsLike(t, validateOutputPath(root, "..", &client.TreeOutput{}), "directory traversal")
			assert.ErrIsLike(t, validateOutputPath(root, "../escaped.txt", &client.TreeOutput{}), "directory traversal")
			assert.ErrIsLike(t, validateOutputPath(root, "../../escaped.txt", &client.TreeOutput{}), "directory traversal")
			assert.ErrIsLike(t, validateOutputPath(root, "a/../../escaped.txt", &client.TreeOutput{}), "directory traversal")
			if runtime.GOOS == "windows" {
				assert.ErrIsLike(t, validateOutputPath(root, "..\\escaped.txt", &client.TreeOutput{}), "directory traversal")
				assert.ErrIsLike(t, validateOutputPath(root, "..\\..\\escaped.txt", &client.TreeOutput{}), "directory traversal")
				assert.ErrIsLike(t, validateOutputPath(root, "a\\..\\..\\escaped.txt", &client.TreeOutput{}), "directory traversal")
			}
		})

		t.Run(`Absolute paths rejected`, func(t *ftt.Test) {
			assert.ErrIsLike(t, validateOutputPath(root, "/etc/passwd", &client.TreeOutput{}), "absolute or contains volume name")
			if runtime.GOOS == "windows" {
				assert.ErrIsLike(t, validateOutputPath(root, "\\Windows\\System32", &client.TreeOutput{}), "absolute or contains volume name")
				assert.ErrIsLike(t, validateOutputPath(root, "C:\\foo", &client.TreeOutput{}), "absolute or contains volume name")
				assert.ErrIsLike(t, validateOutputPath(root, "C:/foo", &client.TreeOutput{}), "absolute or contains volume name")
			}
		})
	})
}

func readDumpJSONResult(t testing.TB, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read dump-json: %v", err)
	}
	var res struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		t.Fatalf("failed to unmarshal dump-json: %v", err)
	}
	return res.Result
}

func TestDownloadPathTraversal(t *testing.T) {
	t.Parallel()

	ftt.Run(`Path traversal prevention in cas download`, t, func(t *ftt.Test) {
		testEnv, cleanup := fakes.NewTestEnv(t)
		t.Cleanup(cleanup)

		c, err := testEnv.Server.NewTestClient(t.Context())
		assert.NoErr(t, err)

		testParentDir := t.TempDir()
		downloadDir := filepath.Join(testParentDir, "download_target")
		assert.NoErr(t, os.Mkdir(downloadDir, 0o700))

		verifyNoEscape := func(t testing.TB) {
			t.Helper()
			entries, err := os.ReadDir(testParentDir)
			assert.NoErr(t, err)
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				names = append(names, e.Name())
			}
			assert.That(t, names, should.Match([]string{"download_target"}))
		}

		t.Run(`Reject DirectoryNode with ".." name`, func(t *ftt.Test) {
			childDir := &repb.Directory{}
			childDg, err := c.WriteProto(t.Context(), childDir)
			assert.NoErr(t, err)

			rootDir := &repb.Directory{
				Directories: []*repb.DirectoryNode{
					{
						Name:   "..",
						Digest: childDg.ToProto(),
					},
				},
			}
			rootDg, err := c.WriteProto(t.Context(), rootDir)
			assert.NoErr(t, err)

			dumpJSONPath := filepath.Join(t.TempDir(), "dump.json")
			dr := downloadRun{
				digest:   rootDg.String(),
				dir:      downloadDir,
				dumpJSON: dumpJSONPath,
			}
			dr.commonFlags.Init(&testAuthFlags{testEnv: testEnv})

			err = dr.doDownload(t.Context())
			assert.ErrIsLike(t, err, "in CAS tree")
			assert.That(t, readDumpJSONResult(t, dumpJSONPath), should.Equal("arguments_invalid"))
			verifyNoEscape(t)
		})

		t.Run(`Reject FileNode with "../../escaped.txt"`, func(t *ftt.Test) {
			blob := []byte("malicious file content")
			blobDg, err := c.WriteBlob(t.Context(), blob)
			assert.NoErr(t, err)

			rootDir := &repb.Directory{
				Files: []*repb.FileNode{
					{
						Name:   "../../escaped.txt",
						Digest: blobDg.ToProto(),
					},
				},
			}
			rootDg, err := c.WriteProto(t.Context(), rootDir)
			assert.NoErr(t, err)

			dumpJSONPath := filepath.Join(t.TempDir(), "dump.json")
			dr := downloadRun{
				digest:   rootDg.String(),
				dir:      downloadDir,
				dumpJSON: dumpJSONPath,
			}
			dr.commonFlags.Init(&testAuthFlags{testEnv: testEnv})

			err = dr.doDownload(t.Context())
			assert.ErrIsLike(t, err, "in CAS tree")
			assert.That(t, readDumpJSONResult(t, dumpJSONPath), should.Equal("arguments_invalid"))
			verifyNoEscape(t)
		})

		t.Run(`Reject FileNode with leading slash or absolute path`, func(t *ftt.Test) {
			blob := []byte("malicious file content")
			blobDg, err := c.WriteBlob(t.Context(), blob)
			assert.NoErr(t, err)

			rootDir := &repb.Directory{
				Files: []*repb.FileNode{
					{
						Name:   "/escaped_abs.txt",
						Digest: blobDg.ToProto(),
					},
				},
			}
			rootDg, err := c.WriteProto(t.Context(), rootDir)
			assert.NoErr(t, err)

			dumpJSONPath := filepath.Join(t.TempDir(), "dump.json")
			dr := downloadRun{
				digest:   rootDg.String(),
				dir:      downloadDir,
				dumpJSON: dumpJSONPath,
			}
			dr.commonFlags.Init(&testAuthFlags{testEnv: testEnv})

			err = dr.doDownload(t.Context())
			assert.ErrIsLike(t, err, "in CAS tree")
			assert.That(t, readDumpJSONResult(t, dumpJSONPath), should.Equal("arguments_invalid"))
			verifyNoEscape(t)
		})

		t.Run(`Reject mixed duplicate digest payload aiming to bypass checks via copyFiles`, func(t *ftt.Test) {
			blob := []byte("duplicate shared content")
			blobDg, err := c.WriteBlob(t.Context(), blob)
			assert.NoErr(t, err)

			rootDir := &repb.Directory{
				Files: []*repb.FileNode{
					{
						Name:   "good.txt",
						Digest: blobDg.ToProto(),
					},
					{
						Name:   "../../escaped_dup.txt",
						Digest: blobDg.ToProto(),
					},
				},
			}
			rootDg, err := c.WriteProto(t.Context(), rootDir)
			assert.NoErr(t, err)

			dumpJSONPath := filepath.Join(t.TempDir(), "dump.json")
			dr := downloadRun{
				digest:   rootDg.String(),
				dir:      downloadDir,
				dumpJSON: dumpJSONPath,
			}
			dr.commonFlags.Init(&testAuthFlags{testEnv: testEnv})

			err = dr.doDownload(t.Context())
			assert.ErrIsLike(t, err, "in CAS tree")
			assert.That(t, readDumpJSONResult(t, dumpJSONPath), should.Equal("arguments_invalid"))
			verifyNoEscape(t)
		})

		t.Run(`Reject SymlinkNode targeting outside root`, func(t *ftt.Test) {
			rootDir := &repb.Directory{
				Symlinks: []*repb.SymlinkNode{
					{
						Name:   "../evil_symlink",
						Target: "some_target",
					},
				},
			}
			rootDg, err := c.WriteProto(t.Context(), rootDir)
			assert.NoErr(t, err)

			dumpJSONPath := filepath.Join(t.TempDir(), "dump.json")
			dr := downloadRun{
				digest:   rootDg.String(),
				dir:      downloadDir,
				dumpJSON: dumpJSONPath,
			}
			dr.commonFlags.Init(&testAuthFlags{testEnv: testEnv})

			err = dr.doDownload(t.Context())
			assert.ErrIsLike(t, err, "in CAS tree")
			assert.That(t, readDumpJSONResult(t, dumpJSONPath), should.Equal("arguments_invalid"))
			verifyNoEscape(t)
		})
	})
}

func TestSinkHardening(t *testing.T) {
	t.Parallel()

	ftt.Run(`createDirectories rejects escaping paths`, t, func(t *ftt.Test) {
		testParentDir := t.TempDir()
		downloadDir := filepath.Join(testParentDir, "download_target")
		assert.NoErr(t, os.Mkdir(downloadDir, 0o700))

		t.Run(`empty directory`, func(t *ftt.Test) {
			outputs := map[string]*client.TreeOutput{
				"../evil_dir": {IsEmptyDirectory: true},
			}
			err := createDirectories(t.Context(), downloadDir, outputs)
			assert.ErrIsLike(t, err, "escapes root")
		})

		t.Run(`file parent directory`, func(t *ftt.Test) {
			outputs := map[string]*client.TreeOutput{
				"../evil_dir/file.txt": {IsEmptyDirectory: false},
			}
			err := createDirectories(t.Context(), downloadDir, outputs)
			assert.ErrIsLike(t, err, "escapes root")
		})

		entries, err := os.ReadDir(testParentDir)
		assert.NoErr(t, err)
		assert.That(t, len(entries), should.Equal(1))
		assert.That(t, entries[0].Name(), should.Equal("download_target"))
	})

	ftt.Run(`copyFiles rejects escaping paths`, t, func(t *ftt.Test) {
		testParentDir := t.TempDir()
		downloadDir := filepath.Join(testParentDir, "download_target")
		assert.NoErr(t, os.Mkdir(downloadDir, 0o700))

		d := digest.Digest{Hash: "abc", Size: 10}

		t.Run(`escaped dst`, func(t *ftt.Test) {
			dsts := []*client.TreeOutput{
				{Path: "../evil_dst.txt", Digest: d},
			}
			srcs := map[digest.Digest]*client.TreeOutput{
				d: {Path: "good_src.txt", Digest: d},
			}
			err := copyFiles(t.Context(), dsts, srcs, downloadDir)
			assert.ErrIsLike(t, err, "escapes root")
		})

		t.Run(`escaped src`, func(t *ftt.Test) {
			dsts := []*client.TreeOutput{
				{Path: "good_dst.txt", Digest: d},
			}
			srcs := map[digest.Digest]*client.TreeOutput{
				d: {Path: "../evil_src.txt", Digest: d},
			}
			err := copyFiles(t.Context(), dsts, srcs, downloadDir)
			assert.ErrIsLike(t, err, "escapes root")
		})
	})
}
