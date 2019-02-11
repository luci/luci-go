// Copyright 2015 The LUCI Authors.
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
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.chromium.org/luci/common/data/text/units"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/isolatedclient/isolatedfake"

	. "github.com/smartystreets/goconvey/convey"
)

// Input to a PushDirectory test case: the given nodes (with respect to a blacklist) are
// isolated and the server contents are validated against the expected files specified
// within the nodes.
type testCase struct {
	// Name of the test case.
	name string

	// List of file nodes.
	nodes []fileNode

	// List of file patterns to ignore.
	blacklist []string
}

// Represents a file in a tree with relevant isolated metadata.
type fileNode struct {
	// The relative path to a file within the directory.
	relPath string

	// The contents of the file.
	contents string

	// The expected isolated.File to be constructed.
	expectedFile isolated.File

	// Whether we expect the file to be ignored, per a given blacklist.
	expectIgnored bool

	// Symlink information about the file.
	symlink *symlinkData
}

// Data about any symlinking related to the file.
type symlinkData struct {
	// Source of the link.
	src string

	// Name of the link.
	name string

	// Whether the link's source is in the directory being pushed.
	inTree bool
}

func TestPushDirectory(t *testing.T) {
	t.Parallel()
	mode := os.FileMode(0600)
	if runtime.GOOS == "windows" {
		mode = os.FileMode(0666)
	}

	// TODO(maruel): Move these variables so we can iterate over multiple
	// namespaces.
	namespace := isolatedclient.DefaultNamespace
	h := isolated.GetHash(namespace)
	basicFile := func(contents string) isolated.File {
		return isolated.BasicFile(isolated.HashBytes(h, []byte(contents)), int(mode), int64(len(contents)))
	}
	expectValidPush := func(t *testing.T, root string, nodes []fileNode, blacklist []string) {
		server := isolatedfake.New()
		ts := httptest.NewServer(server)
		defer ts.Close()
		a := New(context.Background(), isolatedclient.New(nil, nil, ts.URL, namespace, nil, nil), nil)

		for _, node := range nodes {
			if node.symlink != nil {
				continue
			}
			fullPath := filepath.Join(root, node.relPath)
			So(ioutil.WriteFile(fullPath, []byte(node.contents), mode), ShouldBeNil)
		}

		item := PushDirectory(a, root, "", blacklist)
		So(item.Error(), ShouldBeNil)
		So(item.DisplayName, ShouldResemble, filepath.Base(root)+".isolated")
		item.WaitForHashed()
		So(a.Close(), ShouldBeNil)

		expected := map[string]map[isolated.HexDigest]string{namespace: {}}
		expectedMisses := 1
		expectedBytesPushed := 0
		files := make(map[string]isolated.File)
		for _, node := range nodes {
			if node.expectIgnored {
				continue
			}
			files[node.relPath] = node.expectedFile
			if node.symlink != nil && node.symlink.inTree {
				continue
			}
			expectedBytesPushed += int(len(node.contents))
			expectedMisses++
			digest := isolated.HashBytes(h, []byte(node.contents))
			expected[namespace][digest] = node.contents
		}

		isolatedData := isolated.Isolated{
			Algo:    "sha-1",
			Files:   files,
			Version: isolated.IsolatedFormatVersion,
		}
		encoded, err := json.Marshal(isolatedData)
		So(err, ShouldBeNil)
		isolatedEncoded := string(encoded) + "\n"
		isolatedHash := isolated.HashBytes(h, []byte(isolatedEncoded))
		expected[namespace][isolatedHash] = isolatedEncoded
		expectedBytesPushed += len(isolatedEncoded)

		actual := map[string]map[isolated.HexDigest]string{}
		for n, c := range server.Contents() {
			actual[n] = map[isolated.HexDigest]string{}
			for k, v := range c {
				actual[n][k] = string(v)
			}
		}
		So(actual, ShouldResemble, expected)
		So(item.Digest(), ShouldResemble, isolatedHash)

		stats := a.Stats()
		So(stats.TotalMisses(), ShouldResemble, expectedMisses)
		So(stats.TotalBytesPushed(), ShouldResemble, units.Size(expectedBytesPushed))
		So(stats.TotalHits(), ShouldResemble, 0)
		So(stats.TotalBytesHits(), ShouldResemble, units.Size(0))
		So(server.Error(), ShouldBeNil)
	}

	cases := []testCase{
		{
			name: "Can push a directory",
			nodes: []fileNode{
				{
					relPath:      "a",
					contents:     "foo",
					expectedFile: basicFile("foo"),
				},
				{
					relPath:      filepath.Join("b", "c"),
					contents:     "bar",
					expectedFile: basicFile("bar"),
				},
			},
		},
		{
			name: "Can push with two identical files",
			nodes: []fileNode{
				{
					relPath:      "a",
					contents:     "foo",
					expectedFile: basicFile("foo"),
				},
				{
					relPath:      filepath.Join("b"),
					contents:     "foo",
					expectedFile: basicFile("foo"),
				},
			},
		},
		{
			name: "Can push with a blacklist",
			nodes: []fileNode{
				{
					relPath:      "a",
					contents:     "foo",
					expectedFile: basicFile("foo"),
				},
				{
					relPath:      filepath.Join("b", "c"),
					contents:     "bar",
					expectedFile: basicFile("bar"),
				},
				{
					relPath:       filepath.Join("ignored1", "d"),
					contents:      "ignore me",
					expectedFile:  basicFile("ignore me"),
					expectIgnored: true,
				},
				{
					relPath:       filepath.Join("also", "ignored2"),
					contents:      "ignore me too",
					expectedFile:  basicFile("ignore me too"),
					expectIgnored: true,
				},
			},
			blacklist: []string{"ignored1", filepath.Join("*", "ignored2")},
		},
	}

	if runtime.GOOS != "windows" {
		otherDir, err := ioutil.TempDir("", "archiver")
		if err != nil {
			t.Fail()
		}
		defer func() {
			if err := os.RemoveAll(otherDir); err != nil {
				t.Fail()
			}
		}()

		outOfTreeContents := "I am out of the tree"
		outOfTreeFile := filepath.Join(otherDir, "out", "of", "tree")
		if err := os.MkdirAll(filepath.Dir(outOfTreeFile), 0700); err != nil {
			t.Fail()
		}
		if err := ioutil.WriteFile(outOfTreeFile, []byte(outOfTreeContents), mode); err != nil {
			t.Fail()
		}

		cases = append(cases,
			testCase{
				name: "Can push with a symlinked file in-tree",
				nodes: []fileNode{
					{
						relPath:      "a",
						contents:     "foo",
						expectedFile: basicFile("foo"),
					},
					{
						relPath:      "b",
						expectedFile: isolated.SymLink("a"),
						symlink: &symlinkData{
							src:    "a",
							name:   filepath.Join("$ROOT", "b"),
							inTree: true,
						},
					},
				},
			},
			testCase{
				name: "Can push with a symlinked file out-of-tree",
				nodes: []fileNode{
					{
						relPath:      "a",
						contents:     "foo",
						expectedFile: basicFile("foo"),
					},
					{
						relPath:      "b",
						contents:     outOfTreeContents,
						expectedFile: basicFile(outOfTreeContents),
						symlink: &symlinkData{
							src:    outOfTreeFile,
							name:   filepath.Join("$ROOT", "b"),
							inTree: false,
						},
					},
				},
			},
			testCase{
				name: "Can push with a symlinked directory out-of-tree",
				nodes: []fileNode{
					{
						relPath:      filepath.Join("of", "tree"),
						contents:     outOfTreeContents,
						expectedFile: basicFile(outOfTreeContents),
						symlink: &symlinkData{
							src:    filepath.Join(otherDir, "out"),
							name:   filepath.Join("$ROOT", "d"),
							inTree: false,
						},
					},
				},
			},
			testCase{
				name: "Can push with a symlinked directory out-of-tree and a blacklist",
				nodes: []fileNode{
					{
						relPath:       filepath.Join("of", "tree"),
						expectIgnored: true,
						symlink: &symlinkData{
							src:    filepath.Join(otherDir, "out"),
							name:   filepath.Join("$ROOT", "d"),
							inTree: false,
						},
					},
				},
				blacklist: []string{filepath.Join("*", "tree")},
			},
		)
	}

	for _, testCase := range cases {
		Convey(testCase.name, t, func() {
			// Setup temporary directory.
			tmpDir, err := ioutil.TempDir("", "archiver")
			So(err, ShouldBeNil)
			defer func() {
				if err := os.RemoveAll(tmpDir); err != nil {
					t.Fatal(err)
				}
			}()
			for _, node := range testCase.nodes {
				fullPath := filepath.Join(tmpDir, node.relPath)
				if node.symlink == nil {
					So(os.MkdirAll(filepath.Dir(fullPath), 0700), ShouldBeNil)
				} else {
					linkname := strings.Replace(node.symlink.name, "$ROOT", tmpDir, 1)
					So(os.Symlink(node.symlink.src, linkname), ShouldBeNil)
				}
			}
			expectValidPush(t, tmpDir, testCase.nodes, testCase.blacklist)
		})
	}
}
