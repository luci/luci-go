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
	"io/ioutil"
	"net/http"
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"
)

type mockHTTPClient struct{}

func (c *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	r := ioutil.NopCloser(bytes.NewReader([]byte("success")))
	return &http.Response{
		StatusCode: 200,
		Body:       r,
	}, nil
}

func TestNewArtifactRequest(t *testing.T) {
	t.Parallel()
	name := "this is an artifact"
	token := "this is an update token"
	content := "the test passed"
	contentType := "test/output"
	// the hash of "the test passed"
	hash := "sha256:e5d2956e29776b1bca33ff1572bf5ca457cabfb8c370852dbbfcea29953178d2"
	uploader := NewArtifactUploader(&mockHTTPClient{}, "example.org")

	Convey("NewArtifactRequest", t, func() {
		Convey("works with Artifact_FilePath", func() {
			art := testArtifactWithFile(func(f *os.File) {
				_, err := f.Write([]byte(content))
				So(err, ShouldBeNil)
			})
			art.ContentType = contentType
			defer os.Remove(art.GetFilePath())
			req, err := uploader.NewRequest(name, art, token)
			So(err, ShouldBeNil)

			// check the headers
			So(req.ContentLength, ShouldEqual, len(content))
			So(req.Header.Get("Content-Hash"), ShouldEqual, hash)
			So(req.Header.Get("Content-Type"), ShouldEqual, contentType)
			So(req.Header.Get("Update-Token"), ShouldEqual, token)

			// the body should be still readable
			b, err := ioutil.ReadAll(req.Body)
			So(err, ShouldBeNil)
			So(len(b), ShouldEqual, len(content))
		})

		Convey("returns an error if file-open fails", func() {
			art := &sinkpb.Artifact{Body: &sinkpb.Artifact_FilePath{FilePath: "do not exist"}}
			_, err := uploader.NewRequest(name, art, token)
			So(err, ShouldNotBeNil)
		})

		Convey("works with Artifact_Contents", func() {
			art := testArtifactWithContents([]byte(content))
			art.ContentType = contentType
			req, err := uploader.NewRequest(name, art, token)
			So(err, ShouldBeNil)

			// check the headers
			So(req.ContentLength, ShouldEqual, len(content))
			So(req.Header.Get("Content-Hash"), ShouldEqual, hash)
			So(req.Header.Get("Content-Type"), ShouldEqual, contentType)
			So(req.Header.Get("Update-Token"), ShouldEqual, token)

			// the body should be still readable
			b, err := ioutil.ReadAll(req.Body)
			So(err, ShouldBeNil)
			So(len(b), ShouldEqual, len(content))
		})
	})
}
