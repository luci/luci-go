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
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/fakes"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/golang/protobuf/proto"
	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/server/router"
)

const testInstance = "projects/test-proj/instances/default_instance"

func TestHandlers(t *testing.T) {
	t.Parallel()

	Convey("InstallHandlers", t, func() {
		r := router.New()
		InstallHandlers(r, nil)
		srv := httptest.NewServer(r)

		resp, err := http.Get(srv.URL)
		So(err, ShouldBeNil)
		defer resp.Body.Close()
		So(resp.StatusCode, ShouldEqual, http.StatusOK)
	})

	Convey("treeHandler", t, func() {
		Convey("Not Found", func() {
			srv, _ := setupServerWithFakeCAS(t)

			url := fmt.Sprintf(
				"%s/%s/blobs/%s/%d/tree", srv.URL, testInstance, "123", 1)
			resp, err := http.Get(url)
			So(err, ShouldBeNil)
			defer resp.Body.Close()

			So(resp.StatusCode, ShouldEqual, http.StatusNotFound)
		})

		Convey("Must be Directory", func() {
			srv, cl := setupServerWithFakeCAS(t)

			// upload a blob. but it's not directory.
			bd, err := cl.WriteBlob(context.Background(), []byte{1})

			url := fmt.Sprintf(
				"%s/%s/blobs/%s/%d/tree", srv.URL, testInstance, bd.Hash, bd.Size)
			resp, err := http.Get(url)
			So(err, ShouldBeNil)
			defer resp.Body.Close()

			So(resp.StatusCode, ShouldEqual, http.StatusBadRequest)
		})

		Convey("OK", func() {
			srv, cl := setupServerWithFakeCAS(t)

			// upload a director node.
			d := &repb.Directory{
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

			url := fmt.Sprintf(
				"%s/%s/blobs/%s/%d/tree", srv.URL, testInstance, bd.Hash, bd.Size)
			resp, err := http.Get(url)
			So(err, ShouldBeNil)
			defer resp.Body.Close()

			So(resp.StatusCode, ShouldEqual, http.StatusOK)
			body, err := ioutil.ReadAll(resp.Body)
			So(err, ShouldBeNil)
			// body should contain file name, hash, size.
			So(string(body), ShouldContainSubstring, d.Files[0].Name)
			So(string(body), ShouldContainSubstring, d.Files[0].Digest.Hash)
			So(string(body), ShouldContainSubstring, strconv.FormatInt(d.Files[0].Digest.SizeBytes, 10))
		})
	})

	Convey("getHandler", t, func() {
		Convey("Not Found", func() {
			srv, _ := setupServerWithFakeCAS(t)

			url := fmt.Sprintf(
				"%s/%s/blobs/%s/%d", srv.URL, testInstance, "123", 1)
			resp, err := http.Get(url)
			So(err, ShouldBeNil)
			defer resp.Body.Close()

			So(resp.StatusCode, ShouldEqual, http.StatusNotFound)
		})

		Convey("OK", func() {
			srv, cl := setupServerWithFakeCAS(t)

			// upload a blob. but it's not directory.
			b := []byte{1}
			bd, err := cl.WriteBlob(context.Background(), b)

			url := fmt.Sprintf(
				"%s/%s/blobs/%s/%d", srv.URL, testInstance, bd.Hash, bd.Size)
			resp, err := http.Get(url)
			So(err, ShouldBeNil)
			defer resp.Body.Close()

			So(resp.StatusCode, ShouldEqual, http.StatusOK)
			body, err := ioutil.ReadAll(resp.Body)
			So(err, ShouldBeNil)
			So(body, ShouldResemble, b)
		})
	})
}

// setupServerWithFakeCAS sets up a server with a fake CAS client.
func setupServerWithFakeCAS(t *testing.T) (*httptest.Server, *client.Client) {
	casSrv, err := fakes.NewServer(t)
	So(err, ShouldBeNil)

	cl, err := casSrv.NewTestClient(context.Background())
	So(err, ShouldBeNil)

	r := router.New()

	cc := NewClientCache(context.Background())
	t.Cleanup(func() {
		cc.Clear()
		casSrv.Clear()
	})
	cc.clients[testInstance] = cl // Inject fake client.
	InstallHandlers(r, cc)

	return httptest.NewServer(r), cl
}
