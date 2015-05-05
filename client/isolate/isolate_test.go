// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolate

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/luci/luci-go/client/archiver"
	"github.com/luci/luci-go/client/internal/common"
	"github.com/luci/luci-go/client/isolatedclient"
	"github.com/luci/luci-go/client/isolatedclient/isolatedfake"
	"github.com/luci/luci-go/common/isolated"
	"github.com/maruel/ut"
)

func init() {
	log.SetOutput(ioutil.Discard)
}

func TestReplaceVars(t *testing.T) {
	t.Parallel()

	opts := &ArchiveOptions{PathVariables: common.KeyValVars{"VAR": "wonderful"}}

	// Single replacement.
	r, err := ReplaceVariables("hello <(VAR) world", opts)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, "hello wonderful world", r)

	// Multiple replacement.
	r, err = ReplaceVariables("hello <(VAR) <(VAR) world", opts)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, "hello wonderful wonderful world", r)

	// Replacement of missing variable.
	r, err = ReplaceVariables("hello <(MISSING) world", opts)
	ut.AssertEqual(t, "no value for variable 'MISSING'", err.Error())
}

func TestArchive(t *testing.T) {
	// Create a .isolate file and archive it.
	t.Parallel()
	server := isolatedfake.New()
	ts := httptest.NewServer(server)
	defer ts.Close()
	a := archiver.New(isolatedclient.New(ts.URL, "default-gzip"))

	// Setup temporary directory.
	//   /base/bar
	//   /base/ignored
	//   /foo/baz.isolate
	//   /link -> /base/bar
	// Result:
	//   /baz.isolated
	tmpDir, err := ioutil.TempDir("", "archiver")
	ut.AssertEqual(t, nil, err)
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Fail()
		}
	}()
	baseDir := filepath.Join(tmpDir, "base")
	fooDir := filepath.Join(tmpDir, "foo")
	ut.AssertEqual(t, nil, os.Mkdir(baseDir, 0700))
	ut.AssertEqual(t, nil, os.Mkdir(fooDir, 0700))
	ut.AssertEqual(t, nil, ioutil.WriteFile(filepath.Join(baseDir, "bar"), []byte("foo"), 0600))
	ut.AssertEqual(t, nil, ioutil.WriteFile(filepath.Join(baseDir, "ignored"), []byte("ignored"), 0600))
	isolate := `{
		"variables": {
			"files": [
				"./",
				"../link",
			],
		},
		"conditions": [
			["OS=='amiga'", {
				"variables": {
					"command": ["amiga", "<(EXTRA)"],
				},
			}],
			["OS=='win'", {
				"variables": {
					"command": ["win"],
				},
			}],
		],
	}`
	isolatePath := filepath.Join(fooDir, "baz.isolate")
	ut.AssertEqual(t, nil, ioutil.WriteFile(isolatePath, []byte(isolate), 0600))
	// TODO(maruel): Won't work on Windows.
	ut.AssertEqual(t, nil, os.Symlink(filepath.Join("base", "bar"), filepath.Join(tmpDir, "link")))

	opts := &ArchiveOptions{
		Isolate:         isolatePath,
		Isolated:        filepath.Join(tmpDir, "baz.isolated"),
		Blacklist:       common.Strings{"ignored", "*.isolate"},
		PathVariables:   common.KeyValVars{"VAR": "wonderful"},
		ExtraVariables:  common.KeyValVars{"EXTRA": "really"},
		ConfigVariables: common.KeyValVars{"OS": "amiga"},
	}
	future := Archive(a, baseDir, opts)
	ut.AssertEqual(t, "baz.isolated", future.DisplayName())
	future.WaitForHashed()
	ut.AssertEqual(t, nil, future.Error())
	ut.AssertEqual(t, nil, a.Close())

	//   /base/
	isolatedDirData := isolated.Isolated{
		Algo: "sha-1",
		Files: map[string]isolated.File{
			filepath.Join("base", "bar"): {Digest: "0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33", Mode: newInt(384), Size: newInt64(3)},
		},
		Version: isolated.IsolatedFormatVersion,
	}
	encoded, err := json.Marshal(isolatedDirData)
	ut.AssertEqual(t, nil, err)
	isolatedDirEncoded := string(encoded) + "\n"
	isolatedDirHash := isolated.HashBytes([]byte(isolatedDirEncoded))

	isolatedData := isolated.Isolated{
		Algo:    "sha-1",
		Command: []string{"amiga", "really"},
		Files: map[string]isolated.File{
			"link": {Link: newString(filepath.Join("base", "bar"))},
		},
		Includes:    []isolated.HexDigest{isolatedDirHash},
		RelativeCwd: "base",
		Version:     isolated.IsolatedFormatVersion,
	}
	encoded, err = json.Marshal(isolatedData)
	ut.AssertEqual(t, nil, err)
	isolatedEncoded := string(encoded) + "\n"
	isolatedHash := isolated.HashBytes([]byte(isolatedEncoded))

	expected := map[string]string{
		"0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33": "foo",
		string(isolatedDirHash):                    isolatedDirEncoded,
		string(isolatedHash):                       isolatedEncoded,
	}
	actual := map[string]string{}
	for k, v := range server.Contents() {
		actual[string(k)] = string(v)
		ut.AssertEqualf(t, expected[string(k)], actual[string(k)], "%s: %#v", k, actual[string(k)])
	}
	ut.AssertEqual(t, expected, actual)
	ut.AssertEqual(t, isolatedHash, future.Digest())

	stats := a.Stats()
	ut.AssertEqual(t, []int64{}, stats.Hits)
	misses := int64Slice(stats.Misses)
	sort.Sort(misses)
	ut.AssertEqual(t, int64Slice{3, int64(len(isolatedDirEncoded)), int64(len(isolatedEncoded))}, misses)

	ut.AssertEqual(t, nil, server.Error())
}

// Private stuff.

type int64Slice []int64

func (a int64Slice) Len() int           { return len(a) }
func (a int64Slice) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a int64Slice) Less(i, j int) bool { return a[i] < a[j] }
