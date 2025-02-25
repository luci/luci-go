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

	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

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

	ftt.Run("ArtifactUploader", t, func(t *ftt.Test) {
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

		t.Run("Upload w/ file", func(t *ftt.Test) {
			art := testArtifactWithFile(t, func(f *os.File) {
				_, err := f.Write([]byte(content))
				assert.Loosely(t, err, should.BeNil)
			})
			art.ContentType = contentType
			defer os.Remove(art.GetFilePath())

			t.Run("works", func(t *ftt.Test) {
				ut, err := newUploadTask(name, art, pb.TestStatus_STATUS_UNSPECIFIED)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, uploader.StreamUpload(ctx, ut, token), should.BeNil)

				// validate the request
				sent := <-reqCh
				assert.Loosely(t, sent.URL.String(), should.Equal(fmt.Sprintf("https://example.org/%s", name)))
				assert.Loosely(t, sent.ContentLength, should.Equal(len(content)))
				assert.Loosely(t, sent.Header.Get("Content-Hash"), should.Equal(hash))
				assert.Loosely(t, sent.Header.Get("Content-Type"), should.Equal(contentType))
				assert.Loosely(t, sent.Header.Get("Update-Token"), should.Equal(token))
			})

			t.Run("fails if file doesn't exist", func(t *ftt.Test) {
				ut, err := newUploadTask(name, art, pb.TestStatus_STATUS_UNSPECIFIED)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, os.Remove(art.GetFilePath()), should.BeNil)

				// "no such file or directory"
				assert.Loosely(t, uploader.StreamUpload(ctx, ut, token), should.ErrLike("open "+art.GetFilePath()))
			})
		})

		t.Run("Upload w/ contents", func(t *ftt.Test) {
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

				t.Run(fmt.Sprintf("With artifact content type: %s should upload with content type: %s", tc.contentType, tc.expectedContentType), func(t *ftt.Test) {
					art := testArtifactWithContents([]byte(content))
					art.ContentType = tc.contentType
					ut, err := newUploadTask(name, art, pb.TestStatus_STATUS_UNSPECIFIED)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, uploader.StreamUpload(ctx, ut, token), should.BeNil)

					// validate the request
					sent := <-reqCh
					assert.Loosely(t, sent.URL.String(), should.Equal(fmt.Sprintf("https://example.org/%s", name)))
					assert.Loosely(t, sent.ContentLength, should.Equal(len(content)))
					assert.Loosely(t, sent.Header.Get("Content-Hash"), should.Equal(hash))
					assert.Loosely(t, sent.Header.Get("Content-Type"), should.Equal(tc.expectedContentType))
					assert.Loosely(t, sent.Header.Get("Update-Token"), should.Equal(token))
				})
			}
		})

		t.Run("Upload w/ gcs not supported by stream upload", func(t *ftt.Test) {
			art := testArtifactWithGcs(gcsURI)
			art.ContentType = contentType
			ut, err := newUploadTask(name, art, pb.TestStatus_STATUS_UNSPECIFIED)
			assert.Loosely(t, err, should.BeNil)

			// StreamUpload does not support gcsUri upload
			assert.Loosely(t, uploader.StreamUpload(ctx, ut, token), should.ErrLike("StreamUpload does not support gcsUri upload"))
		})

		t.Run("Batch Upload w/ gcs", func(t *ftt.Test) {
			art := testArtifactWithGcs(gcsURI)
			art.ContentType = contentType
			ut, err := newUploadTask(name, art, pb.TestStatus_STATUS_UNSPECIFIED)
			ut.size = 5 * 1024 * 1024
			assert.Loosely(t, err, should.BeNil)

			b := &buffer.Batch[*uploadTask]{
				Data: []buffer.BatchItem[*uploadTask]{{Item: ut}},
			}

			assert.Loosely(t, uploader.BatchUpload(ctx, b), should.BeNil)
			sent := <-batchReqCh

			assert.Loosely(t, sent.Requests[0].Artifact.ContentType, should.Equal(contentType))
			assert.Loosely(t, sent.Requests[0].Artifact.Contents, should.BeNil)
			assert.Loosely(t, sent.Requests[0].Artifact.SizeBytes, should.Equal(5*1024*1024))
			assert.Loosely(t, sent.Requests[0].Artifact.GcsUri, should.Equal(gcsURI))
		})

		t.Run("Batch Upload w/ gcs with artifact size greater than max batch",
			func(t *ftt.Test) {
				art := testArtifactWithGcs(gcsURI)
				art.ContentType = contentType
				ut, err := newUploadTask(name, art, pb.TestStatus_STATUS_UNSPECIFIED)
				ut.size = 15 * 1024 * 1024
				assert.Loosely(t, err, should.BeNil)

				b := &buffer.Batch[*uploadTask]{
					Data: []buffer.BatchItem[*uploadTask]{{Item: ut}},
				}

				assert.Loosely(t, uploader.BatchUpload(ctx, b), should.ErrLike("an artifact is greater than"))

			})

		t.Run("Batch Upload w/ mixed artifacts",
			func(t *ftt.Test) {
				art1 := testArtifactWithFile(t, func(f *os.File) {
					_, err := f.Write([]byte(content))
					assert.Loosely(t, err, should.BeNil)
				})
				art1.ContentType = contentType
				defer os.Remove(art1.GetFilePath())
				ut1, err := newUploadTask(name, art1, pb.TestStatus_STATUS_UNSPECIFIED)
				assert.Loosely(t, err, should.BeNil)

				art2 := testArtifactWithContents([]byte(content))
				art2.ContentType = contentType
				ut2, err := newUploadTask(name, art2, pb.TestStatus_STATUS_UNSPECIFIED)
				assert.Loosely(t, err, should.BeNil)

				art3 := testArtifactWithGcs(gcsURI)
				art3.ContentType = contentType
				ut3, err := newUploadTask(name, art3, pb.TestStatus_STATUS_UNSPECIFIED)
				ut3.size = 5 * 1024 * 1024
				assert.Loosely(t, err, should.BeNil)

				b := &buffer.Batch[*uploadTask]{
					Data: []buffer.BatchItem[*uploadTask]{{Item: ut1}, {Item: ut2}, {Item: ut3}},
				}

				assert.Loosely(t, uploader.BatchUpload(ctx, b), should.BeNil)
				sent := <-batchReqCh

				assert.Loosely(t, sent.Requests[0].Artifact.ContentType, should.Equal(contentType))
				assert.Loosely(t, sent.Requests[0].Artifact.Contents, should.Match([]byte(content)))
				assert.Loosely(t, sent.Requests[0].Artifact.SizeBytes, should.Equal(len(content)))
				assert.Loosely(t, sent.Requests[0].Artifact.GcsUri, should.BeEmpty)

				assert.Loosely(t, sent.Requests[1].Artifact.ContentType, should.Equal(contentType))
				assert.Loosely(t, sent.Requests[1].Artifact.Contents, should.Match([]byte(content)))
				assert.Loosely(t, sent.Requests[1].Artifact.SizeBytes, should.Equal(len(content)))
				assert.Loosely(t, sent.Requests[1].Artifact.GcsUri, should.BeEmpty)

				assert.Loosely(t, sent.Requests[2].Artifact.ContentType, should.Equal(contentType))
				assert.Loosely(t, sent.Requests[2].Artifact.Contents, should.BeNil)
				assert.Loosely(t, sent.Requests[2].Artifact.SizeBytes, should.Equal(5*1024*1024))
				assert.Loosely(t, sent.Requests[2].Artifact.GcsUri, should.Equal(gcsURI))
			})
	})
}
