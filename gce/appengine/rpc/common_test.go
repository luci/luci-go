// Copyright 2019 The LUCI Authors.
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

package rpc

import (
	"context"
	"testing"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/gce/api/projects/v1"
	"go.chromium.org/luci/gce/appengine/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCommon(t *testing.T) {
	t.Parallel()

	Convey("pageQuery", t, func() {
		c := memory.Use(context.Background())
		datastore.GetTestable(c).AutoIndex(true)
		datastore.GetTestable(c).Consistent(true)
		req := &projects.ListRequest{}
		rsp := &projects.ListResponse{}
		q := datastore.NewQuery(model.ProjectKind)

		Convey("invalid", func() {
			Convey("function", func() {
				Convey("nil", func() {
					So(pageQuery(c, req, rsp, q, nil), ShouldErrLike, "callback must be a function")
				})

				Convey("no inputs", func() {
					f := func() error {
						return nil
					}
					So(pageQuery(c, req, rsp, q, f), ShouldErrLike, "callback function must accept one argument")
				})

				Convey("many inputs", func() {
					f := func(interface{}, datastore.CursorCB) error {
						return nil
					}
					So(pageQuery(c, req, rsp, q, f), ShouldErrLike, "callback function must accept one argument")
				})

				Convey("no outputs", func() {
					f := func(interface{}) {
					}
					So(pageQuery(c, req, rsp, q, f), ShouldErrLike, "callback function must return one value")
				})

				Convey("many outputs", func() {
					f := func(interface{}) (interface{}, error) {
						return nil, nil
					}
					So(pageQuery(c, req, rsp, q, f), ShouldErrLike, "callback function must return one value")
				})
			})
		})
	})
}
