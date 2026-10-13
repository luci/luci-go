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
	"regexp"
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

		newTreeRun := func() treeRun {
			var tr treeRun
			tr.commonFlags.Init(&testAuthFlags{testEnv: testEnv})
			tr.casFlags.Instance = "projects/test/instances/default"
			tr.digest = rootDigest
			return tr
		}

		t.Run("basic tree writing to destination file", func(t *ftt.Test) {
			destFile := filepath.Join(t.TempDir(), "tree.json")
			var buf bytes.Buffer
			tr := newTreeRun()
			tr.destination = destFile
			tr.stdout = &buf

			assert.Loosely(t, tr.parse(nil, nil), should.BeNil)
			assert.Loosely(t, tr.doTree(ctx), should.BeNil)

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
			tr := newTreeRun()
			tr.stdout = &buf

			assert.Loosely(t, tr.parse(nil, nil), should.BeNil)
			assert.Loosely(t, tr.doTree(ctx), should.BeNil)

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

			tr := newTreeRun()
			tr.cacheDir = cacheDir
			tr.cachePolicies = cache.Policies{
				MaxSize:  units.Size(1024 * 1024),
				MaxItems: 10,
			}
			tr.destination = destFile

			// First run populates the cache
			assert.Loosely(t, tr.parse(nil, nil), should.BeNil)
			assert.Loosely(t, tr.doTree(ctx), should.BeNil)

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
			tr2.casFlags.Instance = "projects/test/instances/default"
			tr2.digest = rootDigest
			tr2.cacheDir = cacheDir
			tr2.cachePolicies = tr.cachePolicies
			destFile2 := filepath.Join(t.TempDir(), "tree2.json")
			tr2.destination = destFile2

			assert.Loosely(t, tr2.parse(nil, nil), should.BeNil)
			assert.Loosely(t, tr2.doTree(ctx), should.BeNil)

			data2, err := os.ReadFile(destFile2)
			assert.Loosely(t, err, should.BeNil)

			var res2 TreeResult
			assert.Loosely(t, json.Unmarshal(data2, &res2), should.BeNil)
			assert.Loosely(t, res2.TotalFiles, should.Equal(2))
		})

		t.Run("invalid digest writes to destination", func(t *ftt.Test) {
			destFile := filepath.Join(t.TempDir(), "error.json")
			tr := newTreeRun()
			tr.digest = "invalid-digest"
			tr.destination = destFile

			assert.Loosely(t, tr.parse(nil, nil), should.BeNil)
			assert.Loosely(t, tr.doTree(ctx), should.NotBeNil)

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

			tr := newTreeRun()
			tr.cacheDir = filePath
			tr.destination = destFile

			assert.Loosely(t, tr.parse(nil, nil), should.BeNil)
			assert.Loosely(t, tr.doTree(ctx), should.NotBeNil)

			data, err := os.ReadFile(destFile)
			assert.Loosely(t, err, should.BeNil)

			var result struct {
				Result string `json:"result"`
			}
			assert.Loosely(t, json.Unmarshal(data, &result), should.BeNil)
			assert.Loosely(t, result.Result, should.Equal("io_error"))
		})

		t.Run("tree with -exclude filters files and directories and separates cache keys", func(t *ftt.Test) {
			cacheDir := filepath.Join(t.TempDir(), "exclude_cache")

			// 1. Run with -exclude filtering out "sub" directory
			var bufFiltered bytes.Buffer
			trFiltered := newTreeRun()
			trFiltered.cacheDir = cacheDir
			trFiltered.exclude = []string{`^sub($|/)`}
			trFiltered.stdout = &bufFiltered

			assert.Loosely(t, trFiltered.parse(nil, nil), should.BeNil)
			assert.Loosely(t, trFiltered.doTree(ctx), should.BeNil)

			var resFiltered TreeResult
			assert.Loosely(t, json.Unmarshal(bufFiltered.Bytes(), &resFiltered), should.BeNil)
			assert.Loosely(t, resFiltered.TotalFiles, should.Equal(1))
			assert.Loosely(t, resFiltered.Files["foo.txt"].Size, should.Equal(int64(len("hello world"))))
			_, hasSubBar := resFiltered.Files["sub/bar.txt"]
			assert.Loosely(t, hasSubBar, should.BeFalse)

			// 2. Run unfiltered against the same cacheDir - should not return the filtered cache entry
			var bufUnfiltered bytes.Buffer
			trUnfiltered := newTreeRun()
			trUnfiltered.cacheDir = cacheDir
			trUnfiltered.stdout = &bufUnfiltered

			assert.Loosely(t, trUnfiltered.parse(nil, nil), should.BeNil)
			assert.Loosely(t, trUnfiltered.doTree(ctx), should.BeNil)

			var resUnfiltered TreeResult
			assert.Loosely(t, json.Unmarshal(bufUnfiltered.Bytes(), &resUnfiltered), should.BeNil)
			assert.Loosely(t, resUnfiltered.TotalFiles, should.Equal(2))
			_, hasSubBarUnfiltered := resUnfiltered.Files["sub/bar.txt"]
			assert.Loosely(t, hasSubBarUnfiltered, should.BeTrue)

			// 3. Exclude individual file
			var bufFileExclude bytes.Buffer
			trFileExclude := newTreeRun()
			trFileExclude.exclude = []string{`bar\.txt$`}
			trFileExclude.stdout = &bufFileExclude

			assert.Loosely(t, trFileExclude.parse(nil, nil), should.BeNil)
			assert.Loosely(t, trFileExclude.doTree(ctx), should.BeNil)

			var resFileExclude TreeResult
			assert.Loosely(t, json.Unmarshal(bufFileExclude.Bytes(), &resFileExclude), should.BeNil)
			assert.Loosely(t, resFileExclude.TotalFiles, should.Equal(1))
		})

		t.Run("tree with -compact outputs unindented JSON with short keys", func(t *ftt.Test) {
			var bufCompact bytes.Buffer
			trCompact := newTreeRun()
			trCompact.compact = true
			trCompact.stdout = &bufCompact

			assert.Loosely(t, trCompact.parse(nil, nil), should.BeNil)
			assert.Loosely(t, trCompact.doTree(ctx), should.BeNil)

			raw := bytes.TrimSpace(bufCompact.Bytes())
			assert.Loosely(t, bytes.Contains(raw, []byte("\n")), should.BeFalse)
			assert.Loosely(t, bytes.Contains(raw, []byte(`"size":`)), should.BeFalse)

			var resCompact compactTreeResult
			assert.Loosely(t, json.Unmarshal(raw, &resCompact), should.BeNil)
			assert.Loosely(t, resCompact.TotalFiles, should.Equal(2))
			assert.Loosely(t, resCompact.Files["foo.txt"].Digest, should.NotBeEmpty)
			assert.Loosely(t, resCompact.Files["empty_dir"].IsEmptyDir, should.BeTrue)
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

		t.Run("exclude flag repeatable and validates regex", func(t *ftt.Test) {
			cmd := CmdTree(&testAuthFlags{})
			tr := cmd.CommandRun().(*treeRun)
			tr.casFlags.Instance = "projects/test/instances/default"
			err := tr.Flags.Parse([]string{"-digest", "abc/123", "-exclude", `\.so$`, "-exclude", `(^|/)debug(/|$)`})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tr.parse(nil, []string{}), should.BeNil)
			assert.Loosely(t, len(tr.excludeRegexps), should.Equal(2))

			trBad := initRun()
			trBad.digest = "abc/123"
			trBad.exclude = []string{"[invalid"}
			err = trBad.parse(nil, []string{})
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("invalid -exclude regexp"))
		})

		t.Run("cacheKeyDigest avoids collisions between compact and exclude", func(t *ftt.Test) {
			d := digest.Digest{Hash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Size: 98}
			trDefault := treeRun{}
			trCompact := treeRun{compact: true}
			trExcludeC := treeRun{exclude: []string{"c"}}
			trExcludeAbC := treeRun{exclude: []string{"ab", "c"}}
			trExcludeABc := treeRun{exclude: []string{"a", "bc"}}

			assert.Loosely(t, trDefault.cacheKeyDigest(d), should.Equal(cache.HexDigest(d.Hash)))
			assert.Loosely(t, trCompact.cacheKeyDigest(d), should.NotEqual(trDefault.cacheKeyDigest(d)))
			assert.Loosely(t, trCompact.cacheKeyDigest(d), should.NotEqual(trExcludeC.cacheKeyDigest(d)))
			assert.Loosely(t, trExcludeAbC.cacheKeyDigest(d), should.NotEqual(trExcludeABc.cacheKeyDigest(d)))
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

		res := buildTreeResult(d, outputs, nil)
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

		filtered := buildTreeResult(d, outputs, []*regexp.Regexp{
			regexp.MustCompile(`^(link\.txt|empty_folder|bin/run\.sh)$`),
		})
		assert.Loosely(t, filtered.TotalFiles, should.Equal(1))
		assert.Loosely(t, filtered.TotalBytes, should.Equal(int64(1000)))
		assert.Loosely(t, len(filtered.Files), should.Equal(1))
	})
}
