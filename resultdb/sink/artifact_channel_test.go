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
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/resultdb/proto/v1"
	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"
)

func TestArtifactChannel(t *testing.T) {
	t.Parallel()

	ftt.Run("schedule", t, func(t *ftt.Test) {
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
		createTask := func(name, content string) *uploadTask {
			art := testArtifactWithContents([]byte(content))
			task, err := newUploadTask(name, art)
			assert.Loosely(t, err, should.BeNil)
			return task
		}

		t.Run("with a small artifact", func(t *ftt.Test) {
			task := createTask("invocations/inv/artifacts/art1", "content")
			ac := newArtifactChannel(ctx, &cfg)
			ac.schedule(task)
			ac.closeAndDrain(ctx)

			// The artifact should have been sent to the batch channel.
			assert.Loosely(t, <-batchCh, should.Resemble(&pb.BatchCreateArtifactsRequest{
				Requests: []*pb.CreateArtifactRequest{{
					Parent: "invocations/inv",
					Artifact: &pb.Artifact{
						ArtifactId:  "art1",
						ContentType: task.art.ContentType,
						SizeBytes:   int64(len("content")),
						Contents:    []byte("content"),
					},
				}},
			}))
		})

		t.Run("with multiple, small artifacts", func(t *ftt.Test) {
			cfg.MaxBatchableArtifactSize = 10
			ac := newArtifactChannel(ctx, &cfg)

			t1 := createTask("invocations/inv/artifacts/art1", "1234")
			t2 := createTask("invocations/inv/artifacts/art2", "5678")
			t3 := createTask("invocations/inv/artifacts/art3", "9012")
			ac.schedule(t1)
			ac.schedule(t2)
			ac.schedule(t3)
			ac.closeAndDrain(ctx)

			// The 1st request should contain the first two artifacts.
			assert.Loosely(t, <-batchCh, should.Resemble(&pb.BatchCreateArtifactsRequest{
				Requests: []*pb.CreateArtifactRequest{
					// art1
					{
						Parent: "invocations/inv",
						Artifact: &pb.Artifact{
							ArtifactId:  "art1",
							ContentType: t1.art.ContentType,
							SizeBytes:   int64(len("1234")),
							Contents:    []byte("1234"),
						},
					},
					// art2
					{
						Parent: "invocations/inv",
						Artifact: &pb.Artifact{
							ArtifactId:  "art2",
							ContentType: t2.art.ContentType,
							SizeBytes:   int64(len("5678")),
							Contents:    []byte("5678"),
						},
					},
				},
			}))

			// The 2nd request should only contain the last one.
			assert.Loosely(t, <-batchCh, should.Resemble(&pb.BatchCreateArtifactsRequest{
				Requests: []*pb.CreateArtifactRequest{
					// art3
					{
						Parent: "invocations/inv",
						Artifact: &pb.Artifact{
							ArtifactId:  "art3",
							ContentType: t3.art.ContentType,
							SizeBytes:   int64(len("9012")),
							Contents:    []byte("9012"),
						},
					},
				},
			}))
		})

		t.Run("with a large artifact", func(t *ftt.Test) {
			cfg.MaxBatchableArtifactSize = 10
			ac := newArtifactChannel(ctx, &cfg)

			t1 := createTask("invocations/inv/artifacts/art1", "content-foo-bar")
			ac.schedule(t1)
			ac.closeAndDrain(ctx)

			// The artifact should have been sent to the stream channel.
			req := <-streamCh
			assert.Loosely(t, req, should.NotBeNil)
			assert.Loosely(t, req.URL.String(), should.Equal(
				"https://"+cfg.ArtifactStreamHost+"/invocations/inv/artifacts/art1"))
			body, err := io.ReadAll(req.Body)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, body, should.Resemble([]byte("content-foo-bar")))
		})
	})

}

func TestUploadTask(t *testing.T) {
	t.Parallel()

	ftt.Run("newUploadTask", t, func(t *ftt.Test) {
		name := "invocations/inv/artifacts/art1"
		fArt := testArtifactWithFile(t, func(f *os.File) {
			_, err := f.Write([]byte("content"))
			assert.Loosely(t, err, should.BeNil)
		})
		fArt.ContentType = "plain/text"
		defer os.Remove(fArt.GetFilePath())

		t.Run("works", func(t *ftt.Test) {
			tsk, err := newUploadTask(name, fArt)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tsk, should.Resemble(&uploadTask{art: fArt, artName: name, size: int64(len("content"))}))
		})

		t.Run("fails", func(t *ftt.Test) {
			// stat error
			assert.Loosely(t, os.Remove(fArt.GetFilePath()), should.BeNil)
			_, err := newUploadTask(name, fArt)
			assert.Loosely(t, err, should.ErrLike("querying file info"))

			// is a directory
			path, err := os.MkdirTemp("", "foo")
			assert.Loosely(t, err, should.BeNil)
			defer os.RemoveAll(path)
			fArt.Body.(*sinkpb.Artifact_FilePath).FilePath = path
			_, err = newUploadTask(name, fArt)
			assert.Loosely(t, err, should.ErrLike("is a directory"))
		})
	})

	ftt.Run("CreateRequest", t, func(t *ftt.Test) {
		name := "invocations/inv/tests/t1/results/r1/artifacts/a1"
		fArt := testArtifactWithFile(t, func(f *os.File) {
			_, err := f.Write([]byte("content"))
			assert.Loosely(t, err, should.BeNil)
		})
		fArt.ContentType = "plain/text"
		defer os.Remove(fArt.GetFilePath())
		ut, err := newUploadTask(name, fArt)
		assert.Loosely(t, err, should.BeNil)

		t.Run("Updates the content type when it is missing", func(t *ftt.Test) {
			fArt.ContentType = ""
			req, err := ut.CreateRequest()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, req.Artifact.ContentType, should.Equal("text/plain"))
		})

		t.Run("Fails when the artifact name is too long", func(t *ftt.Test) {
			artifactID := strings.Repeat("a", 600)
			name := "invocations/inv/tests/t1/results/r1/artifacts/" + artifactID
			ut, err := newUploadTask(name, fArt)
			assert.Loosely(t, err, should.BeNil)

			_, err = ut.CreateRequest()
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("works", func(t *ftt.Test) {
			req, err := ut.CreateRequest()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, req, should.Match(&pb.CreateArtifactRequest{
				Parent: "invocations/inv/tests/t1/results/r1",
				Artifact: &pb.Artifact{
					ArtifactId:  "a1",
					ContentType: "plain/text",
					SizeBytes:   int64(len("content")),
					Contents:    []byte("content"),
				},
			}))
		})

		t.Run("fails", func(t *ftt.Test) {
			// the artifact content changed.
			assert.Loosely(t, os.WriteFile(fArt.GetFilePath(), []byte("surprise!!"), 0), should.BeNil)
			_, err := ut.CreateRequest()
			assert.Loosely(t, err, should.ErrLike("the size of the artifact contents changed"))

			// the file no longer exists.
			assert.Loosely(t, os.Remove(fArt.GetFilePath()), should.BeNil)
			_, err = ut.CreateRequest()
			assert.Loosely(t, err, should.ErrLike("open "+fArt.GetFilePath())) // no such file or directory
		})
	})
}
