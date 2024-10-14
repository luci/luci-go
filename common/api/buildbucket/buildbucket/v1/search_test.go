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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSearch(t *testing.T) {
	t.Parallel()

	ftt.Run("Search", t, func(c *ftt.Test) {
		ctx := context.Background()

		// Mock buildbucket server.
		responses := []struct {
			body            LegacyApiSearchResponseMessage
			transientErrors int
		}{
			{
				body: LegacyApiSearchResponseMessage{
					Builds: []*LegacyApiCommonBuildMessage{
						{Id: 1},
						{Id: 2},
					},
					NextCursor: "id>2",
				},
			},
			{
				body: LegacyApiSearchResponseMessage{
					Builds: []*LegacyApiCommonBuildMessage{
						{Id: 3},
						{Id: 4},
					},
					NextCursor: "id>4",
				},
			},
			{
				body: LegacyApiSearchResponseMessage{
					Builds: []*LegacyApiCommonBuildMessage{
						{Id: 5},
					},
				},
			},
		}
		var requests []http.Request
		var prevCursor string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests = append(requests, *r)
			assert.Loosely(c, r.URL.Query().Get("start_cursor"), should.Equal(prevCursor))
			res := &responses[0]
			if res.transientErrors > 0 {
				res.transientErrors--
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			responses = responses[1:]
			err := json.NewEncoder(w).Encode(res.body)
			assert.Loosely(c, err, should.BeNil)
			prevCursor = res.body.NextCursor
		}))
		defer server.Close()

		client, err := New(&http.Client{})
		assert.Loosely(c, err, should.BeNil)
		client.BasePath = server.URL

		c.Run("Run until finished", func(c *ftt.Test) {
			builds := make(chan *LegacyApiCommonBuildMessage, 5)
			cursor, err := client.Search().Run(builds, 0, nil)
			assert.Loosely(c, err, should.BeNil)
			for id := 1; id <= 5; id++ {
				assert.Loosely(c, (<-builds).Id, should.Equal(id))
			}
			assert.Loosely(c, cursor, should.BeEmpty)
		})
		c.Run("Run until ctx is cancelled", func(c *ftt.Test) {
			ctx, cancel := context.WithCancel(ctx)
			builds := make(chan *LegacyApiCommonBuildMessage)
			go func() {
				<-builds
				<-builds
				<-builds
				cancel()
			}()
			_, err := client.Search().Context(ctx).Run(builds, 0, nil)
			assert.Loosely(c, err, should.Equal(context.Canceled))
		})
		c.Run("Run with a partial response", func(c *ftt.Test) {
			builds := make(chan *LegacyApiCommonBuildMessage, 5)
			cursor, err := client.Search().Fields("builds(id)").Run(builds, 0, nil)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, len(requests), should.BeGreaterThan(0))
			assert.Loosely(c, requests[0].FormValue("fields"), should.Equal("builds(id),next_cursor"))
			assert.Loosely(c, cursor, should.BeEmpty)
		})

		c.Run("Fetch until finished", func(c *ftt.Test) {
			builds, cursor, err := client.Search().Fetch(0, nil)
			assert.Loosely(c, err, should.BeNil)
			for i, b := range builds {
				assert.Loosely(c, b.Id, should.Equal(i+1))
			}
			assert.Loosely(c, cursor, should.BeEmpty)
		})
		c.Run("Fetch until finished with transient errors", func(c *ftt.Test) {
			responses[0].transientErrors = 2
			builds, cursor, err := client.Search().Fetch(0, nil)
			assert.Loosely(c, err, should.BeNil)
			for i, b := range builds {
				assert.Loosely(c, b.Id, should.Equal(i+1))
			}
			assert.Loosely(c, cursor, should.BeEmpty)
		})

		c.Run("Fetch 3", func(c *ftt.Test) {
			builds, cursor, err := client.Search().Fetch(3, nil)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, builds, should.HaveLength(3))
			assert.Loosely(c, cursor, should.Equal("id>4"))
		})
	})
}
