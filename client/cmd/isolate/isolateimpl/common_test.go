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

package isolateimpl

import (
	"context"
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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestElideNestedPaths(t *testing.T) {
	t.Parallel()
	doElision := func(deps []string) []string {
		// Ignore OS-dependent path sep
		return elideNestedPaths(deps, "/")
	}

	ftt.Run(`Mixed`, t, func(t *ftt.Test) {
		deps := []string{
			"ab/foo",
			"ab/",
			"foo",
			"b/c/",
			"b/a",
			"b/c/a",
			"ab/cd/",
		}
		assert.Loosely(t, doElision(deps), should.Resemble([]string{"ab/", "b/a", "b/c/", "foo"}))
	})

	ftt.Run(`All files`, t, func(t *ftt.Test) {
		deps := []string{
			"ab/foo",
			"ab/cd/foo",
			"foo",
			"ab/bar",
		}
		assert.Loosely(t, doElision(deps), should.Resemble([]string{"ab/bar", "ab/cd/foo", "ab/foo", "foo"}))
	})

	ftt.Run(`Cousin paths`, t, func(t *ftt.Test) {
		deps := []string{
			"ab/foo", // This is a file
			"ab/cd/",
			"ab/ef/",
			"ab/bar",
		}
		assert.Loosely(t, doElision(deps), should.Resemble([]string{"ab/bar", "ab/cd/", "ab/ef/", "ab/foo"}))
	})

	ftt.Run(`Interesting dirs`, t, func(t *ftt.Test) {
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
		assert.Loosely(t, doElision(deps), should.Resemble([]string{"a/b/", "a/bc/", "a/bcd/", "a/c/"}))
	})

	ftt.Run(`Interesting files`, t, func(t *ftt.Test) {
		deps := []string{
			"a/b",
			"a/bc",
			"a/bcd",
			"a/c",
		}
		// Make sure "a/b" elides neither "a/bc" nor "a/bcd"
		assert.Loosely(t, doElision(deps), should.Resemble([]string{"a/b", "a/bc", "a/bcd", "a/c"}))
	})
}

func TestUploadToCAS(t *testing.T) {
	ftt.Run(`Top-level Setup`, t, func(t *ftt.Test) {
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

		t.Run(`Basic`, func(t *ftt.Test) {
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

			writeFile(t, tmpDir, "foo", fooContent)
			writeFile(t, tmpDir, "foo2", fooContent)
			writeFile(t, tmpDir, "bar", barContent)
			writeFile(t, tmpDir, "baz", bazContent)
			isol1Path := writeFile(t, tmpDir, "isol1.isolate", []byte(isol1Content))
			isol2Path := writeFile(t, tmpDir, "isol2.isolate", []byte(isol2Content))

			dgs, err := run.uploadToCAS(context.Background(), "", opts, &fakeFlags, nil, &isolate.ArchiveOptions{
				Isolate: isol1Path,
			}, &isolate.ArchiveOptions{
				Isolate: isol2Path,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, dgs, should.HaveLength(2))

			isol1Dir := &repb.Directory{Files: []*repb.FileNode{
				{Name: "bar", Digest: barDg.ToProto()},
				{Name: "foo", Digest: fooDg.ToProto()},
			}}
			isol2Dir := &repb.Directory{Files: []*repb.FileNode{
				{Name: "baz", Digest: bazDg.ToProto()},
				{Name: "foo2", Digest: fooDg.ToProto()},
			}}
			blob, ok := cas.Get(dgs[0])
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, blob, should.Resemble(mustMarshal(t, isol1Dir)))
			blob, ok = cas.Get(dgs[1])
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, blob, should.Resemble(mustMarshal(t, isol2Dir)))

			assert.Loosely(t, cas.BlobWrites(fooDg), should.Equal(1))
			assert.Loosely(t, cas.BlobWrites(barDg), should.Equal(1))
			assert.Loosely(t, cas.BlobWrites(bazDg), should.Equal(1))
		})

		t.Run(`No upload if already on the server`, func(t *ftt.Test) {
			isol1Content := `{
			  'variables': {
			    'files': [
			      'foo',
			      'bar',
			      'baz',
			    ],
			  },
			}`

			writeFile(t, tmpDir, "foo", fooContent)
			writeFile(t, tmpDir, "bar", barContent)
			writeFile(t, tmpDir, "baz", bazContent)
			isol1Path := writeFile(t, tmpDir, "isol1.isolate", []byte(isol1Content))

			// Upload `foo` and `bar` to the server
			cas.Put(fooContent)
			cas.Put(barContent)

			dgs, err := run.uploadToCAS(context.Background(), "", opts, &fakeFlags, nil, &isolate.ArchiveOptions{
				Isolate: isol1Path,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, dgs, should.HaveLength(1))

			isol1Dir := &repb.Directory{Files: []*repb.FileNode{
				{Name: "bar", Digest: barDg.ToProto()},
				{Name: "baz", Digest: bazDg.ToProto()},
				{Name: "foo", Digest: fooDg.ToProto()},
			}}
			blob, ok := cas.Get(dgs[0])
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, blob, should.Resemble(mustMarshal(t, isol1Dir)))

			assert.Loosely(t, cas.BlobWrites(fooDg), should.BeZero)
			assert.Loosely(t, cas.BlobWrites(barDg), should.BeZero)
			assert.Loosely(t, cas.BlobWrites(bazDg), should.Equal(1))
		})

		t.Run(`Filter files`, func(t *ftt.Test) {
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
			assert.Loosely(t, os.Mkdir(filteredDir, 0700), should.BeNil)
			writeFile(t, filteredDir, "foo", fooContent)
			writeFile(t, filteredDir, "foo2", fooContent)
			writeFile(t, tmpDir, "bar", barContent)
			isol1Path := writeFile(t, tmpDir, "isol1.isolate", []byte(isol1Content))

			dgs, err := run.uploadToCAS(context.Background(), "", opts, &fakeFlags, nil, &isolate.ArchiveOptions{
				Isolate:             isol1Path,
				IgnoredPathFilterRe: "filtered/foo",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, dgs, should.HaveLength(1))

			// `foo*` files are filtered away
			isol1Dir := &repb.Directory{Files: []*repb.FileNode{
				{Name: "bar", Digest: barDg.ToProto()},
			}}
			blob, ok := cas.Get(dgs[0])
			assert.Loosely(t, ok, should.BeTrue)

			gotDir := &repb.Directory{}
			assert.Loosely(t, proto.Unmarshal(blob, gotDir), should.BeNil)
			assert.Loosely(t, gotDir, should.Resemble(isol1Dir))

			assert.Loosely(t, cas.BlobWrites(fooDg), should.BeZero)
			assert.Loosely(t, cas.BlobWrites(barDg), should.Equal(1))
		})
	})
}

func mustMarshal(t testing.TB, p proto.Message) []byte {
	t.Helper()
	b, err := proto.Marshal(p)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	return b
}

func writeFile(t testing.TB, dir, name string, content []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	assert.Loosely(t, os.WriteFile(p, content, 0600), should.BeNil, truth.LineContext())
	return p
}
