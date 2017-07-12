// Copyright 2015 The LUCI Authors.
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

package cipd

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUpload(t *testing.T) {
	ctx := makeTestContext()

	Convey("Upload full flow", t, func(c C) {
		storage := mockStorageImpl(c, []expectedHTTPCall{
			{
				Method:  "PUT",
				Path:    "/upl",
				Body:    "01234",
				Headers: http.Header{"Content-Range": []string{"bytes 0-4/13"}},
			},
			{
				Method:  "PUT",
				Path:    "/upl",
				Body:    "56789",
				Headers: http.Header{"Content-Range": []string{"bytes 5-9/13"}},
			},
			// Insert a error.
			{
				Method:  "PUT",
				Path:    "/upl",
				Body:    "abc",
				Headers: http.Header{"Content-Range": []string{"bytes 10-12/13"}},
				Status:  500,
			},
			// Request for uploaded offset #1: failed itself.
			{
				Method:  "PUT",
				Path:    "/upl",
				Body:    "",
				Headers: http.Header{"Content-Range": []string{"bytes */13"}},
				Status:  500,
			},
			// Request for uploaded offset #2: indicates part of data uploaded.
			{
				Method:          "PUT",
				Path:            "/upl",
				Body:            "",
				Headers:         http.Header{"Content-Range": []string{"bytes */13"}},
				Status:          308,
				ResponseHeaders: http.Header{"Range": []string{"bytes=0-7"}},
			},
			// Resume of the upload from returned offset.
			{
				Method:  "PUT",
				Path:    "/upl",
				Body:    "89abc",
				Headers: http.Header{"Content-Range": []string{"bytes 8-12/13"}},
			},
		})
		err := storage.upload(ctx, "http://localhost/upl", bytes.NewReader([]byte("0123456789abc")))
		So(err, ShouldBeNil)
	})
}

func TestDownload(t *testing.T) {
	ctx := makeTestContext()

	Convey("With temp directory", t, func() {
		tempDir, err := ioutil.TempDir("", "cipd_test")
		So(err, ShouldBeNil)
		defer os.RemoveAll(tempDir)
		tempFile := filepath.Join(tempDir, "pkg")

		Convey("Download full flow", func(c C) {
			out, err := os.OpenFile(tempFile, os.O_RDWR|os.O_CREATE, 0666)
			So(err, ShouldBeNil)
			defer out.Close()

			storage := mockStorageImpl(c, []expectedHTTPCall{
				// Simulate a transient error.
				{
					Method: "GET",
					Path:   "/dwn",
					Status: 500,
					Reply:  "error",
				},
				{
					Method: "GET",
					Path:   "/dwn",
					Status: 200,
					Reply:  "file data",
				},
			})
			h := sha1.New()
			err = storage.download(ctx, "http://localhost/dwn", out, h)
			So(err, ShouldBeNil)

			out.Seek(0, os.SEEK_SET)
			fetched, err := ioutil.ReadAll(out)
			So(err, ShouldBeNil)
			So(string(fetched), ShouldEqual, "file data")
			So(hex.EncodeToString(h.Sum(nil)), ShouldEqual, "cfb9e9ea5ee050291bc74c7e51fbe578a9f3bd4d")
		})
	})
}

////////////////////////////////////////////////////////////////////////////////

func mockStorageImpl(c C, expectations []expectedHTTPCall) *storageImpl {
	client := mockClient(c, "", expectations)
	return &storageImpl{
		chunkSize: 5,
		userAgent: client.UserAgent,
		client:    client.AnonymousClient,
	}
}
