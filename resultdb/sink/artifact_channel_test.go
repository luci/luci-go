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

package sink

import (
	"context"
	"io/ioutil"
	"net/http"
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"
)

func TestArtifactChannel(t *testing.T) {
	t.Parallel()

	Convey("schedule", t, func() {
		cfg := testServerConfig("127.0.0.1:123", "secret")
		ctx := context.Background()

		streamCh := make(chan *http.Request, 10)
		cfg.ArtifactStreamClient.Transport = mockTransport(
			func(in *http.Request) (*http.Response, error) {
				streamCh <- in
				return &http.Response{StatusCode: http.StatusNoContent}, nil
			},
		)
		batchCh := make(chan *pb.BatchCreateArtifactsRequest, 10)
		cfg.Recorder.(*mockRecorder).batchCreateArtifacts = func(ctx context.Context, in *pb.BatchCreateArtifactsRequest) (*pb.BatchCreateArtifactsResponse, error) {
			batchCh <- in
			return nil, nil
		}

		Convey("with a small artifact", func() {
			art := testArtifactWithContents([]byte("content"))
			task, err := newUploadTask("invocations/inv/artifacts/art1", art)
			So(err, ShouldBeNil)

			ac := newArtifactChannel(ctx, &cfg)
			ac.schedule(task)
			ac.closeAndDrain(ctx)

			// The artifact should have been sent to the batch channel.
			So(<-batchCh, ShouldResembleProto, &pb.BatchCreateArtifactsRequest{
				Requests: []*pb.CreateArtifactRequest{{
					Parent: "invocations/inv",
					Artifact: &pb.Artifact{
						ArtifactId:  "art1",
						ContentType: art.ContentType,
						SizeBytes:   int64(len("content")),
						Contents:    []byte("content"),
					},
				}},
			})
		})

		Convey("with a large artifact", func() {
			art := testArtifactWithContents([]byte("content-foo-bar"))
			task, err := newUploadTask("invocations/inv/artifacts/art1", art)
			So(err, ShouldBeNil)

			ac := newArtifactChannel(ctx, &cfg)
			// art with > 10 bytes of content will be sent to the stream channel
			ac.maxBatchable = 10
			ac.schedule(task)
			ac.closeAndDrain(ctx)

			// The artifact should have been sent to the stream channel.
			req := <-streamCh
			So(req, ShouldNotBeNil)
			So(req.URL.String(), ShouldEqual,
				"https://"+cfg.ArtifactStreamHost+"/invocations/inv/artifacts/art1")
			body, err := ioutil.ReadAll(req.Body)
			So(err, ShouldBeNil)
			So(body, ShouldResemble, []byte("content-foo-bar"))
		})
	})

}

func TestUploadTask(t *testing.T) {
	t.Parallel()

	Convey("newUploadTask", t, func() {
		name := "invocations/inv/artifacts/art1"
		fArt := testArtifactWithFile(func(f *os.File) {
			_, err := f.Write([]byte("content"))
			So(err, ShouldBeNil)
		})
		fArt.ContentType = "plain/text"
		defer os.Remove(fArt.GetFilePath())

		Convey("works", func() {
			t, err := newUploadTask(name, fArt)
			So(err, ShouldBeNil)
			So(t, ShouldResemble, &uploadTask{art: fArt, artName: name, size: int64(len("content"))})
		})

		Convey("fails", func() {
			// stat error
			So(os.Remove(fArt.GetFilePath()), ShouldBeNil)
			_, err := newUploadTask(name, fArt)
			So(err, ShouldErrLike, "querying file info")

			// is a directory
			path, err := ioutil.TempDir("", "foo")
			So(err, ShouldBeNil)
			defer os.RemoveAll(path)
			fArt.Body.(*sinkpb.Artifact_FilePath).FilePath = path
			_, err = newUploadTask(name, fArt)
			So(err, ShouldErrLike, "is a directory")
		})
	})

	Convey("CreateRequest", t, func() {
		name := "invocations/inv/tests/t1/results/r1/artifacts/a1"
		fArt := testArtifactWithFile(func(f *os.File) {
			_, err := f.Write([]byte("content"))
			So(err, ShouldBeNil)
		})
		fArt.ContentType = "plain/text"
		defer os.Remove(fArt.GetFilePath())
		ut, err := newUploadTask(name, fArt)
		So(err, ShouldBeNil)

		Convey("works", func() {
			req, err := ut.CreateRequest()
			So(err, ShouldBeNil)
			So(req, ShouldResembleProto, &pb.CreateArtifactRequest{
				Parent: "invocations/inv/tests/t1/results/r1",
				Artifact: &pb.Artifact{
					ArtifactId:  "a1",
					ContentType: "plain/text",
					SizeBytes:   int64(len("content")),
					Contents:    []byte("content"),
				},
			})
		})

		Convey("fails", func() {
			// the artifact content changed.
			So(ioutil.WriteFile(fArt.GetFilePath(), []byte("surprise!!"), 0), ShouldBeNil)
			_, err := ut.CreateRequest()
			So(err, ShouldErrLike, "the size of the artifact contents changed")

			// the file no longer exists.
			So(os.Remove(fArt.GetFilePath()), ShouldBeNil)
			_, err = ut.CreateRequest()
			So(err, ShouldErrLike, "open "+fArt.GetFilePath()) // no such file or directory
		})
	})
}
