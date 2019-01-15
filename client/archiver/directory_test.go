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
	"testing"

	"go.chromium.org/luci/client/internal/common"
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

	// A symbolic link source.
	symlink string
}

func TestPushDirectory(t *testing.T) {
	t.Parallel()
	emptyContext := context.Background()

	mode := os.FileMode(0600)
	if common.IsWindows() {
		mode = os.FileMode(0666)
	}

	basicFile := func(contents string) isolated.File {
		return isolated.BasicFile(isolated.HashBytes([]byte(contents)), int(mode), int64(len(contents)))
	}

	expectValidPush := func(t *testing.T, root string, nodes []fileNode, blacklist []string) {
		server := isolatedfake.New()
		ts := httptest.NewServer(server)
		defer ts.Close()
		a := New(emptyContext, isolatedclient.New(nil, nil, ts.URL, isolatedclient.DefaultNamespace, nil, nil), nil)

		for _, node := range nodes {
			if node.symlink != "" {
				continue
			}
			fullPath := filepath.Join(root, node.relPath)
			So(ioutil.WriteFile(fullPath, []byte(node.contents), mode), ShouldBeNil)
		}

		item := PushDirectory(a, root, "", blacklist)
		So(item.DisplayName, ShouldResemble, filepath.Base(root)+".isolated")
		item.WaitForHashed()
		So(a.Close(), ShouldBeNil)

		expectedServerContents := make(map[string]string)
		expectedMisses := 1
		expectedBytesPushed := 0
		files := make(map[string]isolated.File)
		for _, node := range nodes {
			if node.expectIgnored {
				continue
			}
			files[node.relPath] = node.expectedFile
			if node.symlink != "" {
				continue
			}
			expectedBytesPushed += int(len(node.contents))
			expectedMisses += 1
			digest := isolated.HashBytes([]byte(node.contents))
			expectedServerContents[string(digest)] = node.contents
		}

		isolatedData := isolated.Isolated{
			Algo:    "sha-1",
			Files:   files,
			Version: isolated.IsolatedFormatVersion,
		}
		encoded, err := json.Marshal(isolatedData)
		So(err, ShouldBeNil)
		isolatedEncoded := string(encoded) + "\n"
		isolatedHash := isolated.HashBytes([]byte(isolatedEncoded))
		expectedServerContents[string(isolatedHash)] = isolatedEncoded
		expectedBytesPushed += len(isolatedEncoded)

		actualServerContents := map[string]string{}
		for k, v := range server.Contents() {
			actualServerContents[string(k)] = string(v)
		}
		So(actualServerContents, ShouldResemble, expectedServerContents)
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

	if !common.IsWindows() {
		cases = append(cases,
			testCase{
				name: "Can push with a symlinked file",
				nodes: []fileNode{
					{
						relPath:      "a",
						contents:     "foo",
						expectedFile: basicFile("foo"),
					},
					{
						relPath:      "b",
						contents:     "foo",
						expectedFile: isolated.SymLink("a"),
						symlink:      "a",
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
					t.Fail()
				}
			}()
			for _, node := range testCase.nodes {
				fullPath := filepath.Join(tmpDir, node.relPath)
				So(os.MkdirAll(filepath.Dir(fullPath), 0700), ShouldBeNil)
				if node.symlink != "" {
					So(os.Symlink(node.symlink, fullPath), ShouldBeNil)
				}
			}
			expectValidPush(t, tmpDir, testCase.nodes, testCase.blacklist)
		})
	}
}
