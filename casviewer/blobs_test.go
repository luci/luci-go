package casviewer

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/fakes"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/golang/protobuf/proto"
	. "github.com/smartystreets/goconvey/convey"
)

func TestBlobs(t *testing.T) {
	t.Parallel()

	Convey("renderTree", t, func() {
		// Setup CAS server and client.
		casSrv, err := fakes.NewServer(t)
		So(err, ShouldBeNil)
		cl, err := casSrv.NewTestClient(context.Background())
		t.Cleanup(casSrv.Clear)
		So(err, ShouldBeNil)

		ctx := context.Background()
		w := httptest.NewRecorder()

		Convey("Not Found", func() {
			// This blob doesn't exist on CAS.
			bd := digest.NewFromBlob([]byte{1})

			renderTree(ctx, w, cl, &bd)

			So(w.Code, ShouldEqual, http.StatusNotFound)
		})

		Convey("Must be Directory", func() {
			// This blob exists on CAS, but isn't a Directory.
			bd, err := cl.WriteBlob(context.Background(), []byte{1})
			So(err, ShouldBeNil)

			renderTree(ctx, w, cl, &bd)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("OK", func() {
			// Upload a director node.
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

			renderTree(ctx, w, cl, &bd)

			So(w.Code, ShouldEqual, http.StatusOK)
			body, err := ioutil.ReadAll(w.Body)
			So(err, ShouldBeNil)
			// body should contain file name, hash, size.
			So(string(body), ShouldContainSubstring, d.Files[0].Name)
			So(string(body), ShouldContainSubstring, d.Files[0].Digest.Hash)
			So(string(body), ShouldContainSubstring, strconv.FormatInt(d.Files[0].Digest.SizeBytes, 10))
		})
	})

	Convey("returnBlob", t, func() {
		// Setup CAS server and client.
		casSrv, err := fakes.NewServer(t)
		So(err, ShouldBeNil)
		cl, err := casSrv.NewTestClient(context.Background())
		t.Cleanup(casSrv.Clear)
		So(err, ShouldBeNil)

		ctx := context.Background()
		w := httptest.NewRecorder()

		Convey("Not Found", func() {
			// This blob doesn't exist on CAS.
			bd := digest.NewFromBlob([]byte{1})

			returnBlob(ctx, w, cl, &bd)

			So(w.Code, ShouldEqual, http.StatusNotFound)
		})

		Convey("OK", func() {
			// Upload a blob.
			b := []byte{1}
			bd, err := cl.WriteBlob(context.Background(), b)
			So(err, ShouldBeNil)

			returnBlob(ctx, w, cl, &bd)

			So(w.Code, ShouldEqual, http.StatusOK)
			body, err := ioutil.ReadAll(w.Body)
			So(err, ShouldBeNil)
			So(body, ShouldResemble, b)
		})
	})
}
