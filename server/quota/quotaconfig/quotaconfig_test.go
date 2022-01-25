// Copyright 2022 The LUCI Authors.
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

package quotaconfig

import (
	"context"
	"testing"

	pb "go.chromium.org/luci/server/quota/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMemory(t *testing.T) {
	t.Parallel()

	Convey("Memory", t, func() {
		ctx := context.Background()

		Convey("Get", func() {
			m := &Memory{
				policies: map[string]*pb.Policy{
					"project/name": {
						Name:          "name",
						Resources:     1,
						Replenishment: 1,
					},
				},
			}

			Convey("found", func() {
				p, err := m.Get(ctx, "project", "name")
				So(err, ShouldBeNil)
				So(p, ShouldResembleProto, &pb.Policy{
					Name:          "name",
					Resources:     1,
					Replenishment: 1,
				})
			})

			Convey("project not found", func() {
				p, err := m.Get(ctx, "missing", "name")
				So(err, ShouldErrLike, "not found")
				So(p, ShouldBeNil)
			})

			Convey("policy not found", func() {
				p, err := m.Get(ctx, "project", "missing")
				So(err, ShouldErrLike, "not found")
				So(p, ShouldBeNil)
			})

			Convey("immutable", func() {
				p, err := m.Get(ctx, "project", "name")
				So(err, ShouldBeNil)
				So(p, ShouldResembleProto, &pb.Policy{
					Name:          "name",
					Resources:     1,
					Replenishment: 1,
				})

				p.Resources++
				So(p, ShouldResembleProto, &pb.Policy{
					Name:          "name",
					Resources:     2,
					Replenishment: 1,
				})

				p, err = m.Get(ctx, "project", "name")
				So(err, ShouldBeNil)
				So(p, ShouldResembleProto, &pb.Policy{
					Name:          "name",
					Resources:     1,
					Replenishment: 1,
				})
			})
		})
	})
}
