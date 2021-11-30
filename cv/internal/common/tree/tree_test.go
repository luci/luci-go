// Copyright 2021 The LUCI Authors.
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

package tree

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTreeStatesClient(t *testing.T) {
	t.Parallel()

	Convey("FetchLatest", t, func() {
		ctx := context.Background()
		client := &httpClientImpl{
			&http.Client{Transport: &http.Transport{}},
		}
		Convey("Works", func() {
			var actualReqURL string
			mockSrv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				actualReqURL = req.URL.String()
				rw.WriteHeader(200)
				rw.Header().Set("Content-Type", "application/json")
				fmt.Fprint(rw, `{
					"username":"abc@example.com",
					"can_commit_freely":true,
					"general_state":"open",
					"key":1234567,
					"date":"2000-01-02 03:04:05.678910111",
					"message":"tree is open"
				}`)
			}))
			defer mockSrv.Close()
			ts, err := client.FetchLatest(ctx, mockSrv.URL)
			So(err, ShouldBeNil)
			So(ts, ShouldResemble, Status{
				State: Open,
				Since: time.Date(2000, 01, 02, 03, 04, 05, 678910111, time.UTC),
			})
			So(actualReqURL, ShouldEqual, "/current?format=json")
		})
		Convey("Error if http call fails", func() {
			mockSrv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				rw.WriteHeader(500)
			}))
			defer mockSrv.Close()
			_, err := client.FetchLatest(ctx, mockSrv.URL)
			So(err, ShouldErrLike, "received error when calling")
		})
		Convey("Error if response is not JSON", func() {
			mockSrv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				rw.WriteHeader(200)
				rw.Header().Set("Content-Type", "text/plain")
				fmt.Fprint(rw, "I'm not JSON")
			}))
			defer mockSrv.Close()
			_, err := client.FetchLatest(ctx, mockSrv.URL)
			So(err, ShouldErrLike, "failed to unmarshal JSON")
		})
		Convey("Error if data format is invalid", func() {
			mockSrv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				rw.WriteHeader(200)
				rw.Header().Set("Content-Type", "text/plain")
				fmt.Fprint(rw, `{
					"general_state":"open",
					"date":"2000-01-02T03:04:05.678910111Z"
				}`)
			}))
			defer mockSrv.Close()
			_, err := client.FetchLatest(ctx, mockSrv.URL)
			So(err, ShouldErrLike, "failed to parse date")
		})
	})
}
