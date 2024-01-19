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

	pb "go.chromium.org/luci/tree_status/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestPaginator(t *testing.T) {
	Convey("Paginator", t, func() {
		p := &Paginator{
			DefaultPageSize: 50,
			MaxPageSize:     1000,
		}
		Convey("Limit", func() {
			Convey("Valid value", func() {
				actual := p.Limit(40)

				So(actual, ShouldEqual, 40)
			})
			Convey("Zero value", func() {
				actual := p.Limit(0)

				So(actual, ShouldEqual, 50)
			})
			Convey("Negative value", func() {
				actual := p.Limit(-10)

				So(actual, ShouldEqual, 50)
			})
			Convey("Too large value", func() {
				actual := p.Limit(4000)

				So(actual, ShouldEqual, 1000)
			})
		})
		Convey("Token", func() {
			Convey("Is deterministic", func() {
				token1, err := p.NextPageToken(&pb.ListStatusRequest{Parent: "parent"}, 10)
				So(err, ShouldBeNil)
				token2, err := p.NextPageToken(&pb.ListStatusRequest{Parent: "parent"}, 10)
				So(err, ShouldBeNil)
				So(token1, ShouldEqual, token2)
			})

			Convey("Ignores page_size and page_token", func() {
				token1, err := p.NextPageToken(&pb.ListStatusRequest{Parent: "parent", PageSize: 10, PageToken: "abc"}, 10)
				So(err, ShouldBeNil)
				token2, err := p.NextPageToken(&pb.ListStatusRequest{Parent: "parent", PageSize: 20, PageToken: "def"}, 10)
				So(err, ShouldBeNil)
				So(token1, ShouldEqual, token2)
			})

			Convey("Encodes correct offset", func() {
				token, err := p.NextPageToken(&pb.ListStatusRequest{Parent: "parent", PageSize: 10, PageToken: "abc"}, 42)
				So(err, ShouldBeNil)

				offset, err := p.Offset(&pb.ListStatusRequest{Parent: "parent", PageSize: 10, PageToken: token})
				So(err, ShouldBeNil)
				So(offset, ShouldEqual, 42)
			})
		})
		Convey("Offset", func() {
			Convey("Rejects bad token", func() {
				_, err := p.Offset(&pb.ListStatusRequest{Parent: "parent", PageSize: 10, PageToken: "abc"})
				So(err, ShouldErrLike, "illegal base64 data")
			})

			Convey("Decodes correct offset", func() {
				token, err := p.NextPageToken(&pb.ListStatusRequest{Parent: "parent", PageSize: 10, PageToken: "abc"}, 42)
				So(err, ShouldBeNil)

				offset, err := p.Offset(&pb.ListStatusRequest{Parent: "parent", PageSize: 10, PageToken: token})
				So(err, ShouldBeNil)

				So(offset, ShouldEqual, 42)
			})

			Convey("Ignores changed page_size", func() {
				token, err := p.NextPageToken(&pb.ListStatusRequest{Parent: "parent", PageSize: 10, PageToken: "abc"}, 42)
				So(err, ShouldBeNil)

				offset, err := p.Offset(&pb.ListStatusRequest{Parent: "parent", PageSize: 100, PageToken: token})
				So(err, ShouldBeNil)

				So(offset, ShouldEqual, 42)
			})

			Convey("Rejects changes in request", func() {
				token, err := p.NextPageToken(&pb.ListStatusRequest{Parent: "parent", PageSize: 10, PageToken: "abc"}, 42)
				So(err, ShouldBeNil)

				_, err = p.Offset(&pb.ListStatusRequest{Parent: "changed_value", PageSize: 10, PageToken: token})
				So(err, ShouldErrLike, "request message fields do not match page token")
			})
		})
	})
}
