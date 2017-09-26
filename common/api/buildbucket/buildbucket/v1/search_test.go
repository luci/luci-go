// Copyright 2017 The LUCI Authors.
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

package buildbucket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSearch(t *testing.T) {
	t.Parallel()

	Convey("Search", t, func(c C) {
		ctx := context.Background()

		// Mock buildbucket server.
		responses := []struct {
			body            ApiSearchResponseMessage
			transientErrors int
		}{
			{
				body: ApiSearchResponseMessage{
					Builds: []*ApiCommonBuildMessage{
						{Id: 1},
						{Id: 2},
					},
					NextCursor: "id>2",
				},
			},
			{
				body: ApiSearchResponseMessage{
					Builds: []*ApiCommonBuildMessage{
						{Id: 3},
						{Id: 4},
					},
					NextCursor: "id>4",
				},
			},
			{
				body: ApiSearchResponseMessage{
					Builds: []*ApiCommonBuildMessage{
						{Id: 5},
					},
				},
			},
		}
		var prevCursor string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.So(r.URL.Query().Get("start_cursor"), ShouldEqual, prevCursor)
			res := &responses[0]
			if res.transientErrors > 0 {
				res.transientErrors--
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			responses = responses[1:]
			err := json.NewEncoder(w).Encode(res.body)
			c.So(err, ShouldBeNil)
			prevCursor = res.body.NextCursor
		}))
		defer server.Close()

		client, err := New(&http.Client{})
		So(err, ShouldBeNil)
		client.BasePath = server.URL

		Convey("Run until finished", func() {
			builds := make(chan *ApiCommonBuildMessage, 5)
			err := client.Search().Run(builds, 0, nil)
			So(err, ShouldBeNil)
			for id := 1; id <= 5; id++ {
				So((<-builds).Id, ShouldEqual, id)
			}
		})
		Convey("Run until ctx is cancelled", func() {
			ctx, cancel := context.WithCancel(ctx)
			builds := make(chan *ApiCommonBuildMessage)
			go func() {
				<-builds
				<-builds
				<-builds
				cancel()
			}()
			err := client.Search().Context(ctx).Run(builds, 0, nil)
			So(err, ShouldEqual, context.Canceled)
		})
		Convey("Fetch until finished", func() {
			builds, err := client.Search().Fetch(0, nil)
			So(err, ShouldBeNil)
			for i, b := range builds {
				So(b.Id, ShouldEqual, i+1)
			}
		})
		Convey("Fetch until finished with transient errors", func() {
			responses[0].transientErrors = 2
			builds, err := client.Search().Fetch(0, nil)
			So(err, ShouldBeNil)
			for i, b := range builds {
				So(b.Id, ShouldEqual, i+1)
			}
		})

		Convey("Fetch 3", func() {
			builds, err := client.Search().Fetch(3, nil)
			So(err, ShouldBeNil)
			So(builds, ShouldHaveLength, 3)
		})
	})
}
