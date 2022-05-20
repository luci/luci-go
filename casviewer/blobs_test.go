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

package casviewer

import (
	"context"
	"io/ioutil"
	"net/http/httptest"
	"testing"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/fakes"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/templates"
)

func TestBlobs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = templates.Use(ctx, getTemplateBundle("test-version-1"), nil)

	w := httptest.NewRecorder()

	Convey("renderTree", t, func() {
		cl := fakeClient(ctx, t)

		Convey("Not Found", func() {
			// This blob doesn't exist on CAS.
			bd := digest.NewFromBlob([]byte{1})

			err := renderTree(ctx, w, cl, &bd, testInstance)

			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			So(w.Body.String(), ShouldEqual, "")
		})

		Convey("Must be Directory", func() {
			// This blob exists on CAS, but isn't a Directory.
			bd, err := cl.WriteBlob(context.Background(), []byte{1})
			So(err, ShouldBeNil)

			err = renderTree(ctx, w, cl, &bd, testInstance)

			So(grpcutil.Code(err), ShouldEqual, codes.InvalidArgument)
			So(w.Body.String(), ShouldEqual, "")
		})

		Convey("OK", func() {
			// Upload a directory node.
			d := &repb.Directory{
				Directories: []*repb.DirectoryNode{
					{
						Name:   "subDir",
						Digest: digest.NewFromBlob([]byte{}).ToProto(),
					},
				},
				Files: []*repb.FileNode{
					{
						Name:   "foo",
						Digest: digest.NewFromBlob([]byte{1}).ToProto(),
					},
				},
			}
			b, err := proto.Marshal(d)
			So(err, ShouldBeNil)
			bd, err := cl.WriteBlob(context.Background(), b)
			So(err, ShouldBeNil)

			err = renderTree(ctx, w, cl, &bd, testInstance)

			So(err, ShouldBeNil)
			body, err := ioutil.ReadAll(w.Body)
			So(err, ShouldBeNil)
			// body should contain file name, hash, size.
			So(string(body), ShouldContainSubstring, d.Files[0].Name)
			So(string(body), ShouldContainSubstring, d.Files[0].Digest.Hash)
			So(string(body), ShouldContainSubstring, "1 B")
		})
	})

	Convey("returnBlob", t, func() {
		cl := fakeClient(ctx, t)

		Convey("Not Found", func() {
			// This blob doesn't exist on CAS.
			bd := digest.NewFromBlob([]byte{1})

			err := returnBlob(ctx, w, cl, &bd, "")

			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			So(w.Body.String(), ShouldEqual, "")
		})

		Convey("OK", func() {
			// Upload a blob.
			b := []byte{1}
			bd, err := cl.WriteBlob(context.Background(), b)
			So(err, ShouldBeNil)

			err = returnBlob(ctx, w, cl, &bd, "test.txt")

			So(err, ShouldBeNil)
			body, err := ioutil.ReadAll(w.Body)
			So(err, ShouldBeNil)
			So(body, ShouldResemble, b)
		})
	})
}

// fakeClient returns a Client for a fake CAS.
func fakeClient(ctx context.Context, t *testing.T) *client.Client {
	casSrv, err := fakes.NewServer(t)
	So(err, ShouldBeNil)
	t.Cleanup(casSrv.Clear)
	cl, err := casSrv.NewTestClient(ctx)
	So(err, ShouldBeNil)
	t.Cleanup(func() {
		cl.Close() // ignore error.
	})
	return cl
}
