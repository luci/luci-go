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
	"strings"
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

	opts := &ArchiveOptions{PathVariables: map[string]string{"VAR": "wonderful"}}

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
	a := archiver.New(isolatedclient.New(ts.URL, "default-gzip"), nil)

	// Setup temporary directory.
	//   /base/bar
	//   /base/ignored
	//   /foo/baz.isolate
	//   /link -> /base/bar
	// Result:
	//   /baz.isolated
	tmpDir, err := ioutil.TempDir("", "isolate")
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
		'variables': {
			'files': [
				'../base/',
				'../link',
			],
		},
		'conditions': [
			['OS=="amiga"', {
				'variables': {
					'command': ['amiga', '<(EXTRA)'],
				},
			}],
			['OS=="win"', {
				'variables': {
					'command': ['win'],
				},
			}],
		],
	}`
	isolatePath := filepath.Join(fooDir, "baz.isolate")
	ut.AssertEqual(t, nil, ioutil.WriteFile(isolatePath, []byte(isolate), 0600))
	if !common.IsWindows() {
		ut.AssertEqual(t, nil, os.Symlink(filepath.Join("base", "bar"), filepath.Join(tmpDir, "link")))
	} else {
		ut.AssertEqual(t, nil, ioutil.WriteFile(filepath.Join(tmpDir, "link"), []byte("no link on Windows"), 0600))
	}
	opts := &ArchiveOptions{
		Isolate:         isolatePath,
		Isolated:        filepath.Join(tmpDir, "baz.isolated"),
		Blacklist:       common.Strings{"ignored", "*.isolate"},
		PathVariables:   map[string]string{"VAR": "wonderful"},
		ExtraVariables:  map[string]string{"EXTRA": "really"},
		ConfigVariables: map[string]string{"OS": "amiga"},
	}
	future := Archive(a, opts)
	ut.AssertEqual(t, "baz.isolated", future.DisplayName())
	future.WaitForHashed()
	ut.AssertEqual(t, nil, future.Error())
	ut.AssertEqual(t, nil, a.Close())

	mode := 0600
	if common.IsWindows() {
		mode = 0666
	}
	//   /base/
	isolatedDirData := isolated.Isolated{
		Algo: "sha-1",
		Files: map[string]isolated.File{
			filepath.Join("base", "bar"): {Digest: "0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33", Mode: newInt(mode), Size: newInt64(3)},
		},
		Version: isolated.IsolatedFormatVersion,
	}
	encoded, err := json.Marshal(isolatedDirData)
	ut.AssertEqual(t, nil, err)
	isolatedDirEncoded := string(encoded) + "\n"
	isolatedDirHash := isolated.HashBytes([]byte(isolatedDirEncoded))

	isolatedData := isolated.Isolated{
		Algo:        "sha-1",
		Command:     []string{"amiga", "really"},
		Files:       map[string]isolated.File{},
		Includes:    []isolated.HexDigest{isolatedDirHash},
		RelativeCwd: "foo",
		Version:     isolated.IsolatedFormatVersion,
	}
	if !common.IsWindows() {
		isolatedData.Files["link"] = isolated.File{Link: newString(filepath.Join("base", "bar"))}
	} else {
		isolatedData.Files["link"] = isolated.File{Digest: "12339b9756c2994f85c310d560bc8c142a6b79a1", Mode: newInt(0666), Size: newInt64(18)}
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
	if common.IsWindows() {
		expected["12339b9756c2994f85c310d560bc8c142a6b79a1"] = "no link on Windows"
	}
	actual := map[string]string{}
	for k, v := range server.Contents() {
		actual[string(k)] = string(v)
		ut.AssertEqualf(t, expected[string(k)], actual[string(k)], "%s: %#v", k, actual[string(k)])
	}
	ut.AssertEqual(t, expected, actual)
	ut.AssertEqual(t, isolatedHash, future.Digest())

	stats := a.Stats()
	ut.AssertEqual(t, 0, stats.TotalHits())
	ut.AssertEqual(t, common.Size(0), stats.TotalBytesHits())
	if !common.IsWindows() {
		ut.AssertEqual(t, 3, stats.TotalMisses())
		ut.AssertEqual(t, common.Size(3+len(isolatedDirEncoded)+len(isolatedEncoded)), stats.TotalBytesPushed())
	} else {
		ut.AssertEqual(t, 4, stats.TotalMisses())
		ut.AssertEqual(t, common.Size(3+18+len(isolatedDirEncoded)+len(isolatedEncoded)), stats.TotalBytesPushed())
	}

	ut.AssertEqual(t, nil, server.Error())
	digest, err := isolated.HashFile(filepath.Join(tmpDir, "baz.isolated"))
	ut.AssertEqual(t, isolated.DigestItem{isolatedHash, false, int64(len(isolatedEncoded))}, digest)
	ut.AssertEqual(t, nil, err)
}

// Test that if the isolate file is not found, the error is properly propagated.
func TestArchiveFileNotFoundReturnsError(t *testing.T) {
	t.Parallel()
	a := archiver.New(isolatedclient.New("http://unused", "default-gzip"), nil)
	opts := &ArchiveOptions{
		Isolate:  "/this-file-does-not-exist",
		Isolated: "/this-file-doesnt-either",
	}
	future := Archive(a, opts)
	future.WaitForHashed()
	err := future.Error()
	ut.AssertEqual(t, true, strings.HasPrefix(err.Error(), "open /this-file-does-not-exist: "))
	closeErr := a.Close()
	ut.AssertEqual(t, true, closeErr != nil)
	ut.AssertEqual(t, true, strings.HasPrefix(closeErr.Error(), "open /this-file-does-not-exist: "))
}
