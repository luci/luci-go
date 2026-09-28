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
	"bytes"
	"context"
	"crypto"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/fakes"

	"go.chromium.org/luci/common/data/caching/cache"
	"go.chromium.org/luci/common/data/text/units"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/testfs"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestTree(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run(`Upload and tree`, t, func(t *ftt.Test) {
		testEnv, cleanup := fakes.NewTestEnv(t)
		t.Cleanup(cleanup)

		uploaded := t.TempDir()
		layout := map[string]string{
			"foo.txt":     "hello world",
			"sub/bar.txt": "nested file",
			"empty_dir/":  "",
		}
		assert.Loosely(t, testfs.Build(uploaded, layout), should.BeNil)

		// Upload using archiveRun
		var ar archiveRun
		ar.commonFlags.Init(&testAuthFlags{testEnv: testEnv})
		ar.dumpDigest = filepath.Join(t.TempDir(), "digest")
		assert.Loosely(t, ar.paths.Set(uploaded+":."), should.BeNil)
		assert.Loosely(t, ar.doArchive(ctx), should.BeNil)

		digestBytes, err := os.ReadFile(ar.dumpDigest)
		assert.Loosely(t, err, should.BeNil)
		rootDigest := string(digestBytes)

		t.Run("basic tree writing to destination file", func(t *ftt.Test) {
			destFile := filepath.Join(t.TempDir(), "tree.json")
			var buf bytes.Buffer
			var tr treeRun
			tr.commonFlags.Init(&testAuthFlags{testEnv: testEnv})
			tr.digest = rootDigest
			tr.destination = destFile
			tr.stdout = &buf

			err := tr.doTree(ctx)
			assert.Loosely(t, err, should.BeNil)

			data, err := os.ReadFile(destFile)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.Len(), should.BeZero)

			var res TreeResult
			assert.Loosely(t, json.Unmarshal(data, &res), should.BeNil)
			assert.Loosely(t, res.TotalFiles, should.Equal(2))
			assert.Loosely(t, res.TotalBytes, should.Equal(int64(len("hello world")+len("nested file"))))
			assert.Loosely(t, res.Files["foo.txt"].Size, should.Equal(int64(len("hello world"))))
			assert.Loosely(t, res.Files["sub/bar.txt"].Size, should.Equal(int64(len("nested file"))))
			assert.Loosely(t, res.Files["empty_dir"].IsEmptyDir, should.BeTrue)
		})

		t.Run("basic tree to stdout without destination", func(t *ftt.Test) {
			var buf bytes.Buffer
			var tr treeRun
			tr.commonFlags.Init(&testAuthFlags{testEnv: testEnv})
			tr.digest = rootDigest
			tr.stdout = &buf

			err := tr.doTree(ctx)
			assert.Loosely(t, err, should.BeNil)

			var res TreeResult
			assert.Loosely(t, json.Unmarshal(buf.Bytes(), &res), should.BeNil)
			assert.Loosely(t, res.TotalFiles, should.Equal(2))
			assert.Loosely(t, res.TotalBytes, should.Equal(int64(len("hello world")+len("nested file"))))
			assert.Loosely(t, res.Files["foo.txt"].Size, should.Equal(int64(len("hello world"))))
			assert.Loosely(t, res.Files["sub/bar.txt"].Size, should.Equal(int64(len("nested file"))))
			assert.Loosely(t, res.Files["empty_dir"].IsEmptyDir, should.BeTrue)
		})

		t.Run("tree with size-limited cache", func(t *ftt.Test) {
			cacheDir := filepath.Join(t.TempDir(), "tree_cache")
			destFile := filepath.Join(t.TempDir(), "tree.json")

			var tr treeRun
			tr.commonFlags.Init(&testAuthFlags{testEnv: testEnv})
			tr.digest = rootDigest
			tr.cacheDir = cacheDir
			tr.cachePolicies = cache.Policies{
				MaxSize:  units.Size(1024 * 1024),
				MaxItems: 10,
			}
			tr.destination = destFile

			// First run populates the cache
			err := tr.doTree(ctx)
			assert.Loosely(t, err, should.BeNil)

			// Verify item in cache
			diskcache, err := cache.New(tr.cachePolicies, cacheDir, crypto.SHA256)
			assert.Loosely(t, err, should.BeNil)
			defer func() {
				assert.Loosely(t, diskcache.Close(), should.BeNil)
			}()

			d, err := digest.NewFromString(rootDigest)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, diskcache.Touch(cache.HexDigest(d.Hash)), should.BeTrue)

			// Second run hits cache even if test server client is unconfigured
			var tr2 treeRun
			tr2.commonFlags.Init(&testAuthFlags{testEnv: nil})
			tr2.digest = rootDigest
			tr2.cacheDir = cacheDir
			tr2.cachePolicies = tr.cachePolicies
			destFile2 := filepath.Join(t.TempDir(), "tree2.json")
			tr2.destination = destFile2

			err = tr2.doTree(ctx)
			assert.Loosely(t, err, should.BeNil)

			data2, err := os.ReadFile(destFile2)
			assert.Loosely(t, err, should.BeNil)

			var res2 TreeResult
			assert.Loosely(t, json.Unmarshal(data2, &res2), should.BeNil)
			assert.Loosely(t, res2.TotalFiles, should.Equal(2))
		})

		t.Run("invalid digest writes to destination", func(t *ftt.Test) {
			destFile := filepath.Join(t.TempDir(), "error.json")
			var tr treeRun
			tr.commonFlags.Init(&testAuthFlags{testEnv: testEnv})
			tr.digest = "invalid-digest"
			tr.destination = destFile

			err := tr.doTree(ctx)
			assert.Loosely(t, err, should.NotBeNil)

			data, err := os.ReadFile(destFile)
			assert.Loosely(t, err, should.BeNil)

			var result struct {
				Result       string `json:"result"`
				ErrorDetails struct {
					Digest string `json:"digest,omitempty"`
				} `json:"error_details,omitempty"`
			}
			assert.Loosely(t, json.Unmarshal(data, &result), should.BeNil)
			assert.Loosely(t, result.Result, should.Equal("digest_invalid"))
			assert.Loosely(t, result.ErrorDetails.Digest, should.Equal("invalid-digest"))
		})

		t.Run("cache init failure writes to destination", func(t *ftt.Test) {
			filePath := filepath.Join(t.TempDir(), "not_a_dir")
			assert.Loosely(t, os.WriteFile(filePath, []byte("data"), 0600), should.BeNil)
			destFile := filepath.Join(t.TempDir(), "error.json")

			var tr treeRun
			tr.commonFlags.Init(&testAuthFlags{testEnv: testEnv})
			tr.digest = rootDigest
			tr.cacheDir = filePath
			tr.destination = destFile

			err := tr.doTree(ctx)
			assert.Loosely(t, err, should.NotBeNil)

			data, err := os.ReadFile(destFile)
			assert.Loosely(t, err, should.BeNil)

			var result struct {
				Result string `json:"result"`
			}
			assert.Loosely(t, json.Unmarshal(data, &result), should.BeNil)
			assert.Loosely(t, result.Result, should.Equal("io_error"))
		})
	})
}

func TestTreeParse(t *testing.T) {
	t.Parallel()

	ftt.Run(`treeRun parse flags`, t, func(t *ftt.Test) {
		initRun := func() treeRun {
			cmd := CmdTree(&testAuthFlags{})
			tr := cmd.CommandRun().(*treeRun)
			tr.casFlags.Instance = "projects/test/instances/default"
			return *tr
		}

		t.Run("missing digest", func(t *ftt.Test) {
			tr := initRun()
			err := tr.parse(nil, []string{})
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("-digest is required"))
		})

		t.Run("positional argument accepted as digest", func(t *ftt.Test) {
			tr := initRun()
			err := tr.parse(nil, []string{"abc/123"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tr.digest, should.Equal("abc/123"))
		})

		t.Run("extra positional arguments rejected", func(t *ftt.Test) {
			tr := initRun()
			err := tr.parse(nil, []string{"abc/123", "extra"})
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("position arguments not expected"))
		})

		t.Run("flag digest plus positional argument rejected", func(t *ftt.Test) {
			tr := initRun()
			tr.digest = "abc/123"
			err := tr.parse(nil, []string{"def/456"})
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("position arguments not expected"))
		})

		t.Run("cache policies require cache-dir", func(t *ftt.Test) {
			tr := initRun()
			tr.digest = "abc/123"
			tr.cachePolicies = cache.Policies{MaxSize: 100}
			err := tr.parse(nil, []string{})
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("cache-dir is necessary"))
		})

		t.Run("destination flag registered and parsed", func(t *ftt.Test) {
			cmd := CmdTree(&testAuthFlags{})
			tr := cmd.CommandRun().(*treeRun)
			assert.Loosely(t, tr.Flags.Lookup("destination"), should.NotBeNil)
			assert.Loosely(t, tr.Flags.Lookup("dump-json"), should.BeNil)
			assert.Loosely(t, tr.Flags.Lookup("json"), should.BeNil)

			err := tr.Flags.Parse([]string{"-destination", "/tmp/out.json"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tr.destination, should.Equal("/tmp/out.json"))
		})
	})
}

func TestBuildTreeResult(t *testing.T) {
	t.Parallel()

	ftt.Run(`buildTreeResult`, t, func(t *ftt.Test) {
		d := digest.Digest{Hash: "abc", Size: 100}
		outputs := map[string]*client.TreeOutput{
			"file1.txt": {
				Digest:       digest.Digest{Hash: "d1", Size: 1000},
				IsExecutable: false,
			},
			"bin/run.sh": {
				Digest:       digest.Digest{Hash: "d2", Size: 234},
				IsExecutable: true,
			},
			"link.txt": {
				SymlinkTarget: "file1.txt",
			},
			"empty_folder": {
				IsEmptyDirectory: true,
			},
		}

		res := buildTreeResult(d, outputs)
		assert.Loosely(t, res.Digest, should.Equal("abc/100"))
		assert.Loosely(t, res.TotalFiles, should.Equal(2))
		assert.Loosely(t, res.TotalBytes, should.Equal(int64(1234)))
		assert.Loosely(t, res.Files["file1.txt"].Digest, should.Equal("d1/1000"))
		assert.Loosely(t, res.Files["file1.txt"].Size, should.Equal(int64(1000)))
		assert.Loosely(t, res.Files["file1.txt"].IsExecutable, should.BeFalse)
		assert.Loosely(t, res.Files["bin/run.sh"].Digest, should.Equal("d2/234"))
		assert.Loosely(t, res.Files["bin/run.sh"].Size, should.Equal(int64(234)))
		assert.Loosely(t, res.Files["bin/run.sh"].IsExecutable, should.BeTrue)
		assert.Loosely(t, res.Files["link.txt"].SymlinkTarget, should.Equal("file1.txt"))
		assert.Loosely(t, res.Files["empty_folder"].IsEmptyDir, should.BeTrue)
	})
}
