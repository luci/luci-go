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

package cas

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/fakes"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/rexec"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/golang/protobuf/proto"
	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/client/isolate"
	"go.chromium.org/luci/common/testing/testfs"
)

func TestUploader(t *testing.T) {
	fooContent := []byte("foo")
	barContent := []byte("bar")
	bazContent := []byte("baz")
	fooDg := digest.NewFromBlob(fooContent)
	barDg := digest.NewFromBlob(barContent)
	bazDg := digest.NewFromBlob(bazContent)

	t.Parallel()
	Convey(`Basic`, t, testfs.MustWithTempDir(t, "", func(tmpDir string) {
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

		e, cleanup := fakes.NewTestEnv(t)
		defer cleanup()
		up := newUploader(e.Client)
		dgs, err := up.Upload(context.Background(), &isolate.ArchiveOptions{
			Isolate: isol1Path,
		}, &isolate.ArchiveOptions{
			Isolate: isol2Path,
		})
		So(err, ShouldBeNil)
		So(dgs, ShouldHaveLength, 2)

		cas := e.Server.CAS

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
	}))

	Convey(`No upload if already on the server`, t, testfs.MustWithTempDir(t, "", func(tmpDir string) {
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

		e, cleanup := fakes.NewTestEnv(t)
		defer cleanup()
		// Upload `foo` and `bar` to the server
		cas := e.Server.CAS
		cas.Put(fooContent)
		cas.Put(barContent)

		up := newUploader(e.Client)
		dgs, err := up.Upload(context.Background(), &isolate.ArchiveOptions{
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
	}))

	Convey(`Filter files`, t, testfs.MustWithTempDir(t, "", func(tmpDir string) {
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

		e, cleanup := fakes.NewTestEnv(t)
		defer cleanup()

		up := newUploader(e.Client)

		filteredRe := filepath.Join("filtered", "foo")
		if runtime.GOOS == "windows" {
			// Need to escape `\` on Windows
			filteredRe = `filtered\\foo`
		}
		dgs, err := up.Upload(context.Background(), &isolate.ArchiveOptions{
			Isolate:             isol1Path,
			IgnoredPathFilterRe: filteredRe,
		})
		So(err, ShouldBeNil)
		So(dgs, ShouldHaveLength, 1)

		// `foo*` files are filtered away
		isol1Dir := &repb.Directory{Files: []*repb.FileNode{
			{Name: "bar", Digest: barDg.ToProto()},
		}}
		cas := e.Server.CAS
		blob, ok := cas.Get(dgs[0])
		So(ok, ShouldBeTrue)
		So(blob, ShouldResemble, mustMarshal(isol1Dir))

		So(cas.BlobWrites(fooDg), ShouldEqual, 0)
		So(cas.BlobWrites(barDg), ShouldEqual, 1)
	}))
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

func newUploader(c *rexec.Client) *Uploader {
	return NewUploader(c.GrpcClient, WithCache(c.FileMetadataCache))
}
