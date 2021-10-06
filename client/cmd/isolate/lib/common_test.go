// Copyright 2020 The LUCI Authors.
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

package lib

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/cas"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/fakes"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/client/casclient"
	"go.chromium.org/luci/client/isolate"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestElideNestedPaths(t *testing.T) {
	t.Parallel()
	doElision := func(deps []string) []string {
		// Ignore OS-dependent path sep
		return elideNestedPaths(deps, "/")
	}

	Convey(`Mixed`, t, func() {
		deps := []string{
			"ab/foo",
			"ab/",
			"foo",
			"b/c/",
			"b/a",
			"b/c/a",
			"ab/cd/",
		}
		So(doElision(deps), ShouldResemble, []string{"ab/", "b/a", "b/c/", "foo"})
	})

	Convey(`All files`, t, func() {
		deps := []string{
			"ab/foo",
			"ab/cd/foo",
			"foo",
			"ab/bar",
		}
		So(doElision(deps), ShouldResemble, []string{"ab/bar", "ab/cd/foo", "ab/foo", "foo"})
	})

	Convey(`Cousin paths`, t, func() {
		deps := []string{
			"ab/foo", // This is a file
			"ab/cd/",
			"ab/ef/",
			"ab/bar",
		}
		So(doElision(deps), ShouldResemble, []string{"ab/bar", "ab/cd/", "ab/ef/", "ab/foo"})
	})

	Convey(`Interesting dirs`, t, func() {
		deps := []string{
			"a/b/",
			"a/b/c/",
			"a/bc/",
			"a/bc/d/",
			"a/bcd/",
			"a/c/",
		}
		// Make sure:
		// 1. "a/b/" elides "a/b/c/", but not "a/bc/"
		// 2. "a/bc/" elides "a/bc/d/", but not "a/bcd/"
		So(doElision(deps), ShouldResemble, []string{"a/b/", "a/bc/", "a/bcd/", "a/c/"})
	})

	Convey(`Interesting files`, t, func() {
		deps := []string{
			"a/b",
			"a/bc",
			"a/bcd",
			"a/c",
		}
		// Make sure "a/b" elides neither "a/bc" nor "a/bcd"
		So(doElision(deps), ShouldResemble, []string{"a/b", "a/bc", "a/bcd", "a/c"})
	})
}

func TestUploadToCAS(t *testing.T) {
	Convey(`Top-level Setup`, t, func() {
		// We need a top-level Convey so that the fake env is reset for each test cases.
		// See https://github.com/smartystreets/goconvey/wiki/Execution-order
		tmpDir := t.TempDir()
		fooContent := []byte("foo")
		barContent := []byte("bar")
		bazContent := []byte("baz")
		fooDg := digest.NewFromBlob(fooContent)
		barDg := digest.NewFromBlob(barContent)
		bazDg := digest.NewFromBlob(bazContent)
		fakeFlags := casclient.Flags{
			Instance: "foo",
		}
		var opts auth.Options

		e, cleanup := fakes.NewTestEnv(t)
		defer cleanup()
		run := baseCommandRun{
			clientFactory: func(ctx context.Context, addr string, instance string, opts auth.Options, readOnly bool) (*cas.Client, error) {
				conn, err := e.Server.NewClientConn(ctx)
				if err != nil {
					return nil, err
				}
				return cas.NewClientWithConfig(ctx, conn, "instance", casclient.DefaultConfig())
			},
		}
		cas := e.Server.CAS

		Convey(`Basic`, func() {
			isol1Content := `{
			  'variables': {
			    'files': [
			      'foo',
			      'bar',
			    ],
			  },
			}`
			isol2Content := `{
			  'variables': {
			    'files': [
			      'foo2',
			      'baz',
			    ],
			  },
			}`

			writeFile(tmpDir, "foo", fooContent)
			writeFile(tmpDir, "foo2", fooContent)
			writeFile(tmpDir, "bar", barContent)
			writeFile(tmpDir, "baz", bazContent)
			isol1Path := writeFile(tmpDir, "isol1.isolate", []byte(isol1Content))
			isol2Path := writeFile(tmpDir, "isol2.isolate", []byte(isol2Content))

			dgs, err := run.uploadToCAS(context.Background(), "", opts, &fakeFlags, nil, &isolate.ArchiveOptions{
				Isolate: isol1Path,
			}, &isolate.ArchiveOptions{
				Isolate: isol2Path,
			})
			So(err, ShouldBeNil)
			So(dgs, ShouldHaveLength, 2)

			isol1Dir := &repb.Directory{Files: []*repb.FileNode{
				{Name: "bar", Digest: barDg.ToProto()},
				{Name: "foo", Digest: fooDg.ToProto()},
			}}
			isol2Dir := &repb.Directory{Files: []*repb.FileNode{
				{Name: "baz", Digest: bazDg.ToProto()},
				{Name: "foo2", Digest: fooDg.ToProto()},
			}}
			blob, ok := cas.Get(dgs[0])
			So(ok, ShouldBeTrue)
			So(blob, ShouldResemble, mustMarshal(isol1Dir))
			blob, ok = cas.Get(dgs[1])
			So(ok, ShouldBeTrue)
			So(blob, ShouldResemble, mustMarshal(isol2Dir))

			So(cas.BlobWrites(fooDg), ShouldEqual, 1)
			So(cas.BlobWrites(barDg), ShouldEqual, 1)
			So(cas.BlobWrites(bazDg), ShouldEqual, 1)
		})

		Convey(`No upload if already on the server`, func() {
			isol1Content := `{
			  'variables': {
			    'files': [
			      'foo',
			      'bar',
			      'baz',
			    ],
			  },
			}`

			writeFile(tmpDir, "foo", fooContent)
			writeFile(tmpDir, "bar", barContent)
			writeFile(tmpDir, "baz", bazContent)
			isol1Path := writeFile(tmpDir, "isol1.isolate", []byte(isol1Content))

			// Upload `foo` and `bar` to the server
			cas.Put(fooContent)
			cas.Put(barContent)

			dgs, err := run.uploadToCAS(context.Background(), "", opts, &fakeFlags, nil, &isolate.ArchiveOptions{
				Isolate: isol1Path,
			})
			So(err, ShouldBeNil)
			So(dgs, ShouldHaveLength, 1)

			isol1Dir := &repb.Directory{Files: []*repb.FileNode{
				{Name: "bar", Digest: barDg.ToProto()},
				{Name: "baz", Digest: bazDg.ToProto()},
				{Name: "foo", Digest: fooDg.ToProto()},
			}}
			blob, ok := cas.Get(dgs[0])
			So(ok, ShouldBeTrue)
			So(blob, ShouldResemble, mustMarshal(isol1Dir))

			So(cas.BlobWrites(fooDg), ShouldEqual, 0)
			So(cas.BlobWrites(barDg), ShouldEqual, 0)
			So(cas.BlobWrites(bazDg), ShouldEqual, 1)
		})

		Convey(`Filter files`, func() {
			isol1Content := `{
			  'variables': {
			    'files': [
			      'filtered/foo',
			      'filtered/foo2',
			      'bar',
			    ],
			  },
			}`

			filteredDir := filepath.Join(tmpDir, "filtered")
			So(os.Mkdir(filteredDir, 0700), ShouldBeNil)
			writeFile(filteredDir, "foo", fooContent)
			writeFile(filteredDir, "foo2", fooContent)
			writeFile(tmpDir, "bar", barContent)
			isol1Path := writeFile(tmpDir, "isol1.isolate", []byte(isol1Content))

			dgs, err := run.uploadToCAS(context.Background(), "", opts, &fakeFlags, nil, &isolate.ArchiveOptions{
				Isolate:             isol1Path,
				IgnoredPathFilterRe: "filtered/foo",
			})
			So(err, ShouldBeNil)
			So(dgs, ShouldHaveLength, 1)

			// `foo*` files are filtered away
			isol1Dir := &repb.Directory{Files: []*repb.FileNode{
				{Name: "bar", Digest: barDg.ToProto()},
			}}
			blob, ok := cas.Get(dgs[0])
			So(ok, ShouldBeTrue)

			gotDir := &repb.Directory{}
			So(proto.Unmarshal(blob, gotDir), ShouldBeNil)
			So(gotDir, ShouldResembleProto, isol1Dir)

			So(cas.BlobWrites(fooDg), ShouldEqual, 0)
			So(cas.BlobWrites(barDg), ShouldEqual, 1)
		})
	})
}

func mustMarshal(p proto.Message) []byte {
	b, err := proto.Marshal(p)
	So(err, ShouldBeNil)
	return b
}

func writeFile(dir, name string, content []byte) string {
	p := filepath.Join(dir, name)
	So(ioutil.WriteFile(p, content, 0600), ShouldBeNil)
	return p
}
