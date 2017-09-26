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

	"go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSearch(t *testing.T) {
	t.Parallel()

	Convey("Search", t, func(c C) {
		ctx := context.Background()

		// Mock buildbucket server.
		responses := []struct {
			body            buildbucket.ApiSearchResponseMessage
			transientErrors int
		}{
			{
				body: buildbucket.ApiSearchResponseMessage{
					Builds: []*buildbucket.ApiCommonBuildMessage{
						{Id: 1},
						{Id: 2},
					},
					NextCursor: "id>2",
				},
			},
			{
				body: buildbucket.ApiSearchResponseMessage{
					Builds: []*buildbucket.ApiCommonBuildMessage{
						{Id: 3},
						{Id: 4},
					},
					NextCursor: "id>4",
				},
			},
			{
				body: buildbucket.ApiSearchResponseMessage{
					Builds: []*buildbucket.ApiCommonBuildMessage{
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

		client, err := buildbucket.New(&http.Client{})
		So(err, ShouldBeNil)
		client.BasePath = server.URL

		search := client.Search()

		Convey("until finished", func() {
			builds := make(chan *buildbucket.ApiCommonBuildMessage, 5)
			err := Search(ctx, search, builds, nil)
			So(err, ShouldBeNil)
			for id := 1; id <= 5; id++ {
				So((<-builds).Id, ShouldEqual, id)
			}
		})
		Convey("until ctx is cancelled", func() {
			ctx, cancel := context.WithCancel(ctx)
			builds := make(chan *buildbucket.ApiCommonBuildMessage)
			go func() {
				<-builds
				<-builds
				<-builds
				cancel()
			}()
			err := Search(ctx, search, builds, nil)
			So(err, ShouldEqual, context.Canceled)
		})
		Convey("SearchAll until finished", func() {
			builds, err := SearchAll(ctx, search, nil)
			So(err, ShouldBeNil)
			for i, b := range builds {
				So(b.Id, ShouldEqual, i+1)
			}
		})
		Convey("SearchAll until finished with transient errors", func() {
			responses[0].transientErrors = 2
			builds, err := SearchAll(ctx, search, nil)
			So(err, ShouldBeNil)
			for i, b := range builds {
				So(b.Id, ShouldEqual, i+1)
			}
		})
	})
}
