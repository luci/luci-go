// Copyright 2024 The LUCI Authors.
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

package paginator

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	pb "go.chromium.org/luci/tree_status/proto/v1"
)

func TestPaginator(t *testing.T) {
	ftt.Run("Paginator", t, func(t *ftt.Test) {
		p := &Paginator{
			DefaultPageSize: 50,
			MaxPageSize:     1000,
		}
		t.Run("Limit", func(t *ftt.Test) {
			t.Run("Valid value", func(t *ftt.Test) {
				actual := p.Limit(40)

				assert.Loosely(t, actual, should.Equal(40))
			})
			t.Run("Zero value", func(t *ftt.Test) {
				actual := p.Limit(0)

				assert.Loosely(t, actual, should.Equal(50))
			})
			t.Run("Negative value", func(t *ftt.Test) {
				actual := p.Limit(-10)

				assert.Loosely(t, actual, should.Equal(50))
			})
			t.Run("Too large value", func(t *ftt.Test) {
				actual := p.Limit(4000)

				assert.Loosely(t, actual, should.Equal(1000))
			})
		})
		t.Run("Token", func(t *ftt.Test) {
			t.Run("Is deterministic", func(t *ftt.Test) {
				token1, err := p.NextPageToken(&pb.ListStatusRequest{Parent: "parent"}, 10)
				assert.Loosely(t, err, should.BeNil)
				token2, err := p.NextPageToken(&pb.ListStatusRequest{Parent: "parent"}, 10)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, token1, should.Equal(token2))
			})

			t.Run("Ignores page_size and page_token", func(t *ftt.Test) {
				token1, err := p.NextPageToken(&pb.ListStatusRequest{Parent: "parent", PageSize: 10, PageToken: "abc"}, 10)
				assert.Loosely(t, err, should.BeNil)
				token2, err := p.NextPageToken(&pb.ListStatusRequest{Parent: "parent", PageSize: 20, PageToken: "def"}, 10)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, token1, should.Equal(token2))
			})

			t.Run("Encodes correct offset", func(t *ftt.Test) {
				token, err := p.NextPageToken(&pb.ListStatusRequest{Parent: "parent", PageSize: 10, PageToken: "abc"}, 42)
				assert.Loosely(t, err, should.BeNil)

				offset, err := p.Offset(&pb.ListStatusRequest{Parent: "parent", PageSize: 10, PageToken: token})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, offset, should.Equal(42))
			})
		})
		t.Run("Offset", func(t *ftt.Test) {
			t.Run("Rejects bad token", func(t *ftt.Test) {
				_, err := p.Offset(&pb.ListStatusRequest{Parent: "parent", PageSize: 10, PageToken: "abc"})
				assert.Loosely(t, err, should.ErrLike("illegal base64 data"))
			})

			t.Run("Decodes correct offset", func(t *ftt.Test) {
				token, err := p.NextPageToken(&pb.ListStatusRequest{Parent: "parent", PageSize: 10, PageToken: "abc"}, 42)
				assert.Loosely(t, err, should.BeNil)

				offset, err := p.Offset(&pb.ListStatusRequest{Parent: "parent", PageSize: 10, PageToken: token})
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, offset, should.Equal(42))
			})

			t.Run("Ignores changed page_size", func(t *ftt.Test) {
				token, err := p.NextPageToken(&pb.ListStatusRequest{Parent: "parent", PageSize: 10, PageToken: "abc"}, 42)
				assert.Loosely(t, err, should.BeNil)

				offset, err := p.Offset(&pb.ListStatusRequest{Parent: "parent", PageSize: 100, PageToken: token})
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, offset, should.Equal(42))
			})

			t.Run("Rejects changes in request", func(t *ftt.Test) {
				token, err := p.NextPageToken(&pb.ListStatusRequest{Parent: "parent", PageSize: 10, PageToken: "abc"}, 42)
				assert.Loosely(t, err, should.BeNil)

				_, err = p.Offset(&pb.ListStatusRequest{Parent: "changed_value", PageSize: 10, PageToken: token})
				assert.Loosely(t, err, should.ErrLike("request message fields do not match page token"))
			})
		})
	})
}
