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

	"go.chromium.org/luci/config/validation"

	pb "go.chromium.org/luci/server/quota/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestQuotaConfig(t *testing.T) {
	t.Parallel()

	Convey("Memory", t, func() {
		ctx := context.Background()

		Convey("Get", func() {
			m := &Memory{
				policies: map[string]*pb.Policy{
					"name": {
						Name:          "name",
						Resources:     1,
						Replenishment: 1,
					},
				},
			}

			Convey("policy not found", func() {
				p, err := m.Get(ctx, "missing")
				So(err, ShouldEqual, ErrNotFound)
				So(p, ShouldBeNil)
			})

			Convey("found", func() {
				p, err := m.Get(ctx, "name")
				So(err, ShouldBeNil)
				So(p, ShouldResembleProto, &pb.Policy{
					Name:          "name",
					Resources:     1,
					Replenishment: 1,
				})
			})

			Convey("immutable", func() {
				p, err := m.Get(ctx, "name")
				So(err, ShouldBeNil)
				So(p, ShouldResembleProto, &pb.Policy{
					Name:          "name",
					Resources:     1,
					Replenishment: 1,
				})

				p.Resources++

				p, err = m.Get(ctx, "name")
				So(err, ShouldBeNil)
				So(p, ShouldResembleProto, &pb.Policy{
					Name:          "name",
					Resources:     1,
					Replenishment: 1,
				})
			})
		})
	})

	Convey("ValidatePolicy", t, func() {
		ctx := &validation.Context{Context: context.Background()}

		Convey("name", func() {
			Convey("nil", func() {
				ValidatePolicy(ctx, nil)
				err := ctx.Finalize()
				So(err, ShouldNotBeNil)
				So(err.(*validation.Error).Errors, ShouldContainErr, "name must match")
			})

			Convey("empty", func() {
				p := &pb.Policy{}

				ValidatePolicy(ctx, p)
				err := ctx.Finalize()
				So(err, ShouldNotBeNil)
				So(err.(*validation.Error).Errors, ShouldContainErr, "name must match")
			})

			Convey("invalid char", func() {
				p := &pb.Policy{
					Name: "projects/project/users/${name}",
				}

				ValidatePolicy(ctx, p)
				err := ctx.Finalize()
				So(err, ShouldNotBeNil)
				So(err.(*validation.Error).Errors, ShouldContainErr, "name must match")
			})

			Convey("user", func() {
				p := &pb.Policy{
					Name: "projects/project/users/${user}",
				}

				ValidatePolicy(ctx, p)
				So(ctx.Finalize(), ShouldBeNil)
			})

			Convey("len", func() {
				p := &pb.Policy{
					Name: "projects/project/users/user/policies/very-long-policy-name-exceeds-limit",
				}

				ValidatePolicy(ctx, p)
				err := ctx.Finalize()
				So(err, ShouldNotBeNil)
				So(err.(*validation.Error).Errors, ShouldContainErr, "name must not exceed")
			})
		})

		Convey("resources", func() {
			Convey("negative", func() {
				p := &pb.Policy{
					Name:      "name",
					Resources: -1,
				}

				ValidatePolicy(ctx, p)
				err := ctx.Finalize()
				So(err, ShouldNotBeNil)
				So(err.(*validation.Error).Errors, ShouldContainErr, "resources must not be negative")
			})

			Convey("zero", func() {
				p := &pb.Policy{
					Name: "name",
				}

				ValidatePolicy(ctx, p)
				So(ctx.Finalize(), ShouldBeNil)
			})

			Convey("positive", func() {
				p := &pb.Policy{
					Name:      "name",
					Resources: 1,
				}

				ValidatePolicy(ctx, p)
				So(ctx.Finalize(), ShouldBeNil)
			})
		})

		Convey("replenishment", func() {
			Convey("negative", func() {
				p := &pb.Policy{
					Name:          "name",
					Replenishment: -1,
				}

				ValidatePolicy(ctx, p)
				err := ctx.Finalize()

				So(err, ShouldNotBeNil)
				So(err.(*validation.Error).Errors, ShouldContainErr, "replenishment must not be negative")
			})

			Convey("zero", func() {
				p := &pb.Policy{
					Name: "name",
				}

				ValidatePolicy(ctx, p)
				So(ctx.Finalize(), ShouldBeNil)
			})
			Convey("positive", func() {
				p := &pb.Policy{
					Name:          "name",
					Replenishment: 1,
				}

				ValidatePolicy(ctx, p)
				So(ctx.Finalize(), ShouldBeNil)
			})
		})
	})
}
