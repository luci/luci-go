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
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"testing"

	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type mockTransport struct {
	mockFn (func(*http.Request) (*http.Response, error))
}

func (c *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if c.mockFn != nil {
		return c.mockFn(req)
	}
	return &http.Response{
		StatusCode: 200, Body: ioutil.NopCloser(bytes.NewReader([]byte("success"))),
	}, nil
}

func TestArtifactUploader(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	name := "invocations/inv1/tests/t1/results/r1/output"
	token := "this is an update token"
	content := "the test passed"
	contentType := "test/output"
	// the hash of "the test passed"
	hash := "sha256:e5d2956e29776b1bca33ff1572bf5ca457cabfb8c370852dbbfcea29953178d2"
	reqCh := make(chan *http.Request, 1)
	keepReq := func(req *http.Request) (*http.Response, error) {
		reqCh <- req
		return &http.Response{StatusCode: http.StatusNoContent}, nil
	}
	uploader := NewArtifactUploader(&http.Client{Transport: &mockTransport{keepReq}}, "example.org")

	Convey("UploadFromFile", t, func() {
		art := testArtifactWithFile(func(f *os.File) {
			_, err := f.Write([]byte(content))
			So(err, ShouldBeNil)
		})
		defer os.Remove(art.GetFilePath())

		Convey("works", func() {
			err := uploader.UploadFromFile(ctx, name, contentType, art.GetFilePath(), token)
			So(err, ShouldBeNil)

			// validate the request
			sent := <-reqCh
			So(sent.URL.String(), ShouldEqual, fmt.Sprintf("https://example.org/%s", name))
			So(sent.ContentLength, ShouldEqual, len(content))
			So(sent.Header.Get("Content-Hash"), ShouldEqual, hash)
			So(sent.Header.Get("Content-Type"), ShouldEqual, contentType)
			So(sent.Header.Get("Update-Token"), ShouldEqual, token)
		})

		Convey("fails if file-open fails", func() {
			art.Body = &sinkpb.Artifact_FilePath{FilePath: "no_file.txt"}
			err := uploader.UploadFromFile(ctx, name, contentType, art.GetFilePath(), token)
			So(err, ShouldErrLike, "stat no_file.txt")
		})
	})

	Convey("Upload", t, func() {
		err := uploader.Upload(ctx, name, contentType, bytes.NewReader([]byte(content)), token)
		So(err, ShouldBeNil)

		// validate the request
		sent := <-reqCh
		So(sent.URL.String(), ShouldEqual, fmt.Sprintf("https://example.org/%s", name))
		So(sent.ContentLength, ShouldEqual, len(content))
		So(sent.Header.Get("Content-Hash"), ShouldEqual, hash)
		So(sent.Header.Get("Content-Type"), ShouldEqual, contentType)
		So(sent.Header.Get("Update-Token"), ShouldEqual, token)
	})
}
