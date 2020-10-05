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

// Input to a PushDirectory test case: the given nodes are
// isolated and the server contents are validated against the expected files specified
// within the nodes.
type testCase struct {
	// Name of the test case.
	name string

	// List of file nodes.
	nodes []fileNode
}

// Represents a file in a tree with relevant isolated metadata.
type fileNode struct {
	// The relative path to a file within the directory.
	relPath string

	// The contents of the file.
	contents string

	// The expected isolated.File to be constructed.
	expectedFile isolated.File

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
				name: "Can push with a symlinked file in-tree pointing to absolute path",
				nodes: []fileNode{
					{
						relPath:      "first/a",
						contents:     "foo",
						expectedFile: basicFile("foo"),
					},
					{
						relPath:      "second/b",
						expectedFile: isolated.SymLink(filepath.Join("..", "first", "a")),
						symlink: &symlinkData{
							// "second/b" -> "/PATH/TO/ROOT/first/a" (absolute path)
							src:    filepath.Join("$ROOT", "first", "a"),
							name:   filepath.Join("$ROOT", "second", "b"),
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
						relPath:      filepath.Join("d", "of", "tree"),
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
			expectValidPush(t, tmpDir, testCase.nodes, namespace, mode)
		})
	}
}

func TestRelDestOfInTreeSymlink(t *testing.T) {
	t.Parallel()
	// The function is used only when the symlink points to an abs path,
	// so the evaluated directory must also be an absolute path as well.
	evaledDir := t.TempDir()
	cases := []struct {
		name      string
		symlink   string
		evaledSym string
		expected  string
	}{
		{
			name:      "basic",
			symlink:   "link",
			evaledSym: filepath.Join(evaledDir, "bar"),
			expected:  "bar",
		},
		{
			name:      "parent dir",
			symlink:   filepath.Join("sub", "link"),
			evaledSym: filepath.Join(evaledDir, "bar"),
			expected:  filepath.Join("..", "bar"),
		},
		{
			name:      "sibling dir",
			symlink:   filepath.Join("first", "link"),
			evaledSym: filepath.Join(evaledDir, "second", "bar"),
			expected:  filepath.Join("..", "second", "bar"),
		},
	}
	for _, testCase := range cases {
		tc := testCase
		Convey(tc.name, t, func() {
			rel, err := computeRelDestOfInTreeSymlink(tc.symlink, tc.evaledSym, evaledDir)
			So(err, ShouldBeNil)
			So(rel, ShouldResemble, tc.expected)
		})
	}
}

func expectValidPush(t *testing.T, root string, nodes []fileNode, namespace string, mode os.FileMode) {
	h := isolated.GetHash(namespace)
	server := isolatedfake.New()
	ts := httptest.NewServer(server)
	defer ts.Close()
	a := New(context.Background(), isolatedclient.NewClient(ts.URL, isolatedclient.WithNamespace(namespace)), nil)

	// Create the directories and symlinks.
	for _, node := range nodes {
		fullPath := filepath.Join(root, node.relPath)
		if node.symlink == nil {
			So(os.MkdirAll(filepath.Dir(fullPath), 0700), ShouldBeNil)
		} else {
			src := strings.Replace(node.symlink.src, "$ROOT", root, 1)
			linkname := strings.Replace(node.symlink.name, "$ROOT", root, 1)
			So(os.MkdirAll(filepath.Dir(linkname), 0700), ShouldBeNil)
			So(os.Symlink(src, linkname), ShouldBeNil)
		}
	}
	// Write the files.
	for _, node := range nodes {
		if node.symlink != nil {
			continue
		}
		fullPath := filepath.Join(root, node.relPath)
		So(ioutil.WriteFile(fullPath, []byte(node.contents), mode), ShouldBeNil)
	}

	fItems, symItems, err := PushDirectory(a, root, "")
	So(err, ShouldBeNil)
	So(a.Close(), ShouldBeNil)

	expected := map[string]map[isolated.HexDigest]string{namespace: {}}
	expectedMisses := 0
	expectedBytesPushed := 0
	files := make(map[string]isolated.File)
	expectedOrdinaryFiles := 0
	for _, node := range nodes {
		files[node.relPath] = node.expectedFile
		if node.symlink != nil && node.symlink.inTree {
			So(symItems[node.relPath], ShouldResemble, *node.expectedFile.Link)
			continue
		}
		expectedBytesPushed += int(len(node.contents))
		expectedMisses++
		digest := isolated.HashBytes(h, []byte(node.contents))
		expected[namespace][digest] = node.contents
		expectedOrdinaryFiles++
	}
	So(fItems, ShouldHaveLength, expectedOrdinaryFiles)
	for p, i := range fItems {
		So(p.Digest(), ShouldResemble, files[i.RelPath].Digest)
	}

	actual := map[string]map[isolated.HexDigest]string{}
	for n, c := range server.Contents() {
		actual[n] = map[isolated.HexDigest]string{}
		for k, v := range c {
			actual[n][k] = string(v)
		}
	}
	So(actual, ShouldResemble, expected)

	stats := a.Stats()
	So(stats.TotalMisses(), ShouldResemble, expectedMisses)
	So(stats.TotalBytesPushed(), ShouldResemble, units.Size(expectedBytesPushed))
	So(stats.TotalHits(), ShouldResemble, 0)
	So(stats.TotalBytesHits(), ShouldResemble, units.Size(0))
	So(server.Error(), ShouldBeNil)
}
