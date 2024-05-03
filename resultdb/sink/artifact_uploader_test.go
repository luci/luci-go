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
	"fmt"
	"net/http"
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	. "go.chromium.org/luci/common/testing/assertions"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type mockTransport func(*http.Request) (*http.Response, error)

func (c mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return c(req)
}

func TestArtifactUploader(t *testing.T) {
	t.Parallel()

	name := "invocations/inv1/tests/t1/results/r1/artifacts/stderr"
	token := "this is an update token"
	content := "the test passed"
	contentType := "test/output"
	// the hash of "the test passed"
	hash := "sha256:e5d2956e29776b1bca33ff1572bf5ca457cabfb8c370852dbbfcea29953178d2"
	gcsURI := "gs://bucket/foo"

	Convey("ArtifactUploader", t, func() {
		ctx := context.Background()
		reqCh := make(chan *http.Request, 1)
		keepReq := func(req *http.Request) (*http.Response, error) {
			reqCh <- req
			return &http.Response{StatusCode: http.StatusNoContent}, nil
		}
		batchReqCh := make(chan *pb.BatchCreateArtifactsRequest, 1)
		keepBatchReq := func(ctx context.Context, in *pb.BatchCreateArtifactsRequest) (*pb.BatchCreateArtifactsResponse, error) {
			batchReqCh <- in
			return nil, nil
		}
		uploader := &artifactUploader{
			Recorder:     &mockRecorder{batchCreateArtifacts: keepBatchReq},
			StreamClient: &http.Client{Transport: mockTransport(keepReq)},
			StreamHost:   "example.org",
			MaxBatchable: 10 * 1024 * 1024,
		}

		Convey("Upload w/ file", func() {
			art := testArtifactWithFile(func(f *os.File) {
				_, err := f.Write([]byte(content))
				So(err, ShouldBeNil)
			})
			art.ContentType = contentType
			defer os.Remove(art.GetFilePath())

			Convey("works", func() {
				ut, err := newUploadTask(name, art, pb.TestStatus_STATUS_UNSPECIFIED)
				So(err, ShouldBeNil)
				So(uploader.StreamUpload(ctx, ut, token), ShouldBeNil)

				// validate the request
				sent := <-reqCh
				So(sent.URL.String(), ShouldEqual, fmt.Sprintf("https://example.org/%s", name))
				So(sent.ContentLength, ShouldEqual, len(content))
				So(sent.Header.Get("Content-Hash"), ShouldEqual, hash)
				So(sent.Header.Get("Content-Type"), ShouldEqual, contentType)
				So(sent.Header.Get("Update-Token"), ShouldEqual, token)
			})

			Convey("fails if file doesn't exist", func() {
				ut, err := newUploadTask(name, art, pb.TestStatus_STATUS_UNSPECIFIED)
				So(err, ShouldBeNil)
				So(os.Remove(art.GetFilePath()), ShouldBeNil)

				// "no such file or directory"
				So(uploader.StreamUpload(ctx, ut, token), ShouldErrLike, "open "+art.GetFilePath())
			})
		})

		Convey("Upload w/ contents", func() {
			tests := []struct {
				contentType         string
				expectedContentType string
			}{{
				contentType:         contentType,
				expectedContentType: contentType,
			},
				{
					contentType:         "",
					expectedContentType: "text/plain",
				},
			}
			for _, tc := range tests {

				Convey(fmt.Sprintf("With artifact content type: %s should upload with content type: %s", tc.contentType, tc.expectedContentType), func() {
					art := testArtifactWithContents([]byte(content))
					art.ContentType = tc.contentType
					ut, err := newUploadTask(name, art, pb.TestStatus_STATUS_UNSPECIFIED)
					So(err, ShouldBeNil)
					So(uploader.StreamUpload(ctx, ut, token), ShouldBeNil)

					// validate the request
					sent := <-reqCh
					So(sent.URL.String(), ShouldEqual, fmt.Sprintf("https://example.org/%s", name))
					So(sent.ContentLength, ShouldEqual, len(content))
					So(sent.Header.Get("Content-Hash"), ShouldEqual, hash)
					So(sent.Header.Get("Content-Type"), ShouldEqual, tc.expectedContentType)
					So(sent.Header.Get("Update-Token"), ShouldEqual, token)
				})
			}
		})

		Convey("Upload w/ gcs not supported by stream upload", func() {
			art := testArtifactWithGcs(gcsURI)
			art.ContentType = contentType
			ut, err := newUploadTask(name, art, pb.TestStatus_STATUS_UNSPECIFIED)
			So(err, ShouldBeNil)

			// StreamUpload does not support gcsUri upload
			So(uploader.StreamUpload(ctx, ut, token), ShouldErrLike, "StreamUpload does not support gcsUri upload")
		})

		Convey("Batch Upload w/ gcs", func() {
			art := testArtifactWithGcs(gcsURI)
			art.ContentType = contentType
			ut, err := newUploadTask(name, art, pb.TestStatus_STATUS_UNSPECIFIED)
			ut.size = 5 * 1024 * 1024
			So(err, ShouldBeNil)

			b := &buffer.Batch[*uploadTask]{
				Data: []buffer.BatchItem[*uploadTask]{{Item: ut}},
			}

			So(uploader.BatchUpload(ctx, b), ShouldBeNil)
			sent := <-batchReqCh

			So(sent.Requests[0].Artifact.ContentType, ShouldEqual, contentType)
			So(sent.Requests[0].Artifact.Contents, ShouldBeNil)
			So(sent.Requests[0].Artifact.SizeBytes, ShouldEqual, 5*1024*1024)
			So(sent.Requests[0].Artifact.GcsUri, ShouldEqual, gcsURI)
		})

		Convey("Batch Upload w/ gcs with artifact size greater than max batch",
			func() {
				art := testArtifactWithGcs(gcsURI)
				art.ContentType = contentType
				ut, err := newUploadTask(name, art, pb.TestStatus_STATUS_UNSPECIFIED)
				ut.size = 15 * 1024 * 1024
				So(err, ShouldBeNil)

				b := &buffer.Batch[*uploadTask]{
					Data: []buffer.BatchItem[*uploadTask]{{Item: ut}},
				}

				So(uploader.BatchUpload(ctx, b), ShouldErrLike, "an artifact is greater than")

			})

		Convey("Batch Upload w/ mixed artifacts",
			func() {
				art1 := testArtifactWithFile(func(f *os.File) {
					_, err := f.Write([]byte(content))
					So(err, ShouldBeNil)
				})
				art1.ContentType = contentType
				defer os.Remove(art1.GetFilePath())
				ut1, err := newUploadTask(name, art1, pb.TestStatus_STATUS_UNSPECIFIED)
				So(err, ShouldBeNil)

				art2 := testArtifactWithContents([]byte(content))
				art2.ContentType = contentType
				ut2, err := newUploadTask(name, art2, pb.TestStatus_STATUS_UNSPECIFIED)
				So(err, ShouldBeNil)

				art3 := testArtifactWithGcs(gcsURI)
				art3.ContentType = contentType
				ut3, err := newUploadTask(name, art3, pb.TestStatus_STATUS_UNSPECIFIED)
				ut3.size = 5 * 1024 * 1024
				So(err, ShouldBeNil)

				b := &buffer.Batch[*uploadTask]{
					Data: []buffer.BatchItem[*uploadTask]{{Item: ut1}, {Item: ut2}, {Item: ut3}},
				}

				So(uploader.BatchUpload(ctx, b), ShouldBeNil)
				sent := <-batchReqCh

				So(sent.Requests[0].Artifact.ContentType, ShouldEqual, contentType)
				So(sent.Requests[0].Artifact.Contents, ShouldResemble, []byte(content))
				So(sent.Requests[0].Artifact.SizeBytes, ShouldEqual, len(content))
				So(sent.Requests[0].Artifact.GcsUri, ShouldEqual, "")

				So(sent.Requests[1].Artifact.ContentType, ShouldEqual, contentType)
				So(sent.Requests[1].Artifact.Contents, ShouldResemble, []byte(content))
				So(sent.Requests[1].Artifact.SizeBytes, ShouldEqual, len(content))
				So(sent.Requests[1].Artifact.GcsUri, ShouldEqual, "")

				So(sent.Requests[2].Artifact.ContentType, ShouldEqual, contentType)
				So(sent.Requests[2].Artifact.Contents, ShouldBeNil)
				So(sent.Requests[2].Artifact.SizeBytes, ShouldEqual, 5*1024*1024)
				So(sent.Requests[2].Artifact.GcsUri, ShouldEqual, gcsURI)
			})
	})
}
