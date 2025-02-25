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
	"io"
	"net/http/httptest"
	"testing"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/fakes"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/templates"
)

func TestBlobs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = templates.Use(ctx, getTemplateBundle("test-version-1"), nil)

	w := httptest.NewRecorder()

	ftt.Run("renderTree", t, func(t *ftt.Test) {
		cl := fakeClient(ctx, t)

		t.Run("Not Found", func(t *ftt.Test) {
			// This blob doesn't exist on CAS.
			bd := digest.NewFromBlob([]byte{1})

			err := renderTree(ctx, w, cl, &bd, testInstance)

			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, w.Body.String(), should.BeEmpty)
		})

		t.Run("Must be Directory", func(t *ftt.Test) {
			// This blob exists on CAS, but isn't a Directory.
			bd, err := cl.WriteBlob(context.Background(), []byte{1})
			assert.Loosely(t, err, should.BeNil)

			err = renderTree(ctx, w, cl, &bd, testInstance)

			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, w.Body.String(), should.BeEmpty)
		})

		t.Run("OK", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			bd, err := cl.WriteBlob(context.Background(), b)
			assert.Loosely(t, err, should.BeNil)

			err = renderTree(ctx, w, cl, &bd, testInstance)

			assert.Loosely(t, err, should.BeNil)
			body, err := io.ReadAll(w.Body)
			assert.Loosely(t, err, should.BeNil)
			// body should contain file name, hash, size.
			assert.Loosely(t, string(body), should.ContainSubstring(d.Files[0].Name))
			assert.Loosely(t, string(body), should.ContainSubstring(d.Files[0].Digest.Hash))
			assert.Loosely(t, string(body), should.ContainSubstring("1 B"))
		})
	})

	ftt.Run("returnBlob", t, func(t *ftt.Test) {
		cl := fakeClient(ctx, t)

		t.Run("Not Found", func(t *ftt.Test) {
			// This blob doesn't exist on CAS.
			bd := digest.NewFromBlob([]byte{1})

			err := returnBlob(ctx, w, cl, &bd, "")

			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, w.Body.String(), should.BeEmpty)
		})

		t.Run("OK", func(t *ftt.Test) {
			// Upload a blob.
			b := []byte{1}
			bd, err := cl.WriteBlob(context.Background(), b)
			assert.Loosely(t, err, should.BeNil)

			err = returnBlob(ctx, w, cl, &bd, "test.txt")

			assert.Loosely(t, err, should.BeNil)
			body, err := io.ReadAll(w.Body)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, body, should.Match(b))
		})
	})
}

// fakeClient returns a Client for a fake CAS.
func fakeClient(ctx context.Context, t testing.TB) *client.Client {
	casSrv, err := fakes.NewServer(t)
	assert.Loosely(t, err, should.BeNil)
	t.Cleanup(casSrv.Clear)
	cl, err := casSrv.NewTestClient(ctx)
	assert.Loosely(t, err, should.BeNil)
	t.Cleanup(func() {
		cl.Close() // ignore error.
	})
	return cl
}
