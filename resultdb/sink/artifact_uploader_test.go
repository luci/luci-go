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
	. "go.chromium.org/luci/common/testing/assertions"
	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"
)

type mockTransport func(*http.Request) (*http.Response, error)

func (c mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return c(req)
}

func TestArtifactUploader(t *testing.T) {
	t.Parallel()

	name := "invocations/inv1/tests/t1/results/r1/output"
	token := "this is an update token"
	content := "the test passed"
	contentType := "test/output"
	// the hash of "the test passed"
	hash := "sha256:e5d2956e29776b1bca33ff1572bf5ca457cabfb8c370852dbbfcea29953178d2"

	Convey("ArtifactUploader", t, func() {
		ctx := context.Background()
		reqCh := make(chan *http.Request, 1)
		keepReq := func(req *http.Request) (*http.Response, error) {
			reqCh <- req
			return &http.Response{StatusCode: http.StatusNoContent}, nil
		}
		uploader := &artifactUploader{
			Recorder:     &mockRecorder{},
			StreamClient: &http.Client{Transport: mockTransport(keepReq)},
			StreamHost:   "example.org",
		}

		Convey("Upload w/ file", func() {
			Convey("works", func() {
				art := testArtifactWithFile(func(f *os.File) {
					_, err := f.Write([]byte(content))
					So(err, ShouldBeNil)
				})
				art.ContentType = contentType
				defer os.Remove(art.GetFilePath())
				err := uploader.StreamUpload(ctx, &uploadTask{name, art}, token)
				So(err, ShouldBeNil)

				// validate the request
				sent := <-reqCh
				So(sent.URL.String(), ShouldEqual, fmt.Sprintf("https://example.org/%s", name))
				So(sent.ContentLength, ShouldEqual, len(content))
				So(sent.Header.Get("Content-Hash"), ShouldEqual, hash)
				So(sent.Header.Get("Content-Type"), ShouldEqual, contentType)
				So(sent.Header.Get("Update-Token"), ShouldEqual, token)
			})

			Convey("fails if file doesn't exist", func() {
				art := &sinkpb.Artifact{
					Body:        &sinkpb.Artifact_FilePath{FilePath: "never_exist"},
					ContentType: "text/plain",
				}
				err := uploader.StreamUpload(ctx, &uploadTask{name, art}, token)
				So(err, ShouldErrLike, "open never_exist: ") // no such file or directory
			})
		})

		Convey("Upload w/ contents", func() {
			art := testArtifactWithContents([]byte(content))
			art.ContentType = contentType
			err := uploader.StreamUpload(ctx, &uploadTask{name, art}, token)
			So(err, ShouldBeNil)

			// validate the request
			sent := <-reqCh
			So(sent.URL.String(), ShouldEqual, fmt.Sprintf("https://example.org/%s", name))
			So(sent.ContentLength, ShouldEqual, len(content))
			So(sent.Header.Get("Content-Hash"), ShouldEqual, hash)
			So(sent.Header.Get("Content-Type"), ShouldEqual, contentType)
			So(sent.Header.Get("Update-Token"), ShouldEqual, token)
		})
	})
}
