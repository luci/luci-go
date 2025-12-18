// Copyright 2025 The LUCI Authors.
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

package resultdb

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestFetchArtifactContentByURL(t *testing.T) {
	t.Parallel()

	ftt.Run("fetchArtifactContentByURL", t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run("successfully fetches content", func(t *ftt.Test) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, "test artifact content")
			}))
			defer server.Close()

			client := &clientImpl{
				httpClient: server.Client(),
			}

			content, err := client.fetchArtifactContentByURL(ctx, server.URL)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, content, should.Equal("test artifact content"))
		})

		t.Run("handles empty URL", func(t *ftt.Test) {
			client := &clientImpl{
				httpClient: &http.Client{},
			}

			content, err := client.fetchArtifactContentByURL(ctx, "")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("empty fetch URL"))
			assert.Loosely(t, content, should.BeEmpty)
		})

		t.Run("handles HTTP error status", func(t *ftt.Test) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}))
			defer server.Close()

			client := &clientImpl{
				httpClient: server.Client(),
			}

			content, err := client.fetchArtifactContentByURL(ctx, server.URL)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("unexpected status code: 404"))
			assert.Loosely(t, content, should.BeEmpty)
		})

		t.Run("respects size limit", func(t *ftt.Test) {
			// Create a large response (12MB, exceeding the 10MB limit)
			largeContent := make([]byte, 12*1024*1024)
			for i := range largeContent {
				largeContent[i] = 'A'
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write(largeContent)
			}))
			defer server.Close()

			client := &clientImpl{
				httpClient: server.Client(),
			}

			content, err := client.fetchArtifactContentByURL(ctx, server.URL)
			assert.Loosely(t, err, should.BeNil)
			// Content should be truncated to 10MB
			assert.Loosely(t, len(content), should.Equal(10*1024*1024))
		})

		t.Run("handles server error", func(t *ftt.Test) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()

			client := &clientImpl{
				httpClient: server.Client(),
			}

			content, err := client.fetchArtifactContentByURL(ctx, server.URL)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("unexpected status code: 500"))
			assert.Loosely(t, content, should.BeEmpty)
		})
	})
}
