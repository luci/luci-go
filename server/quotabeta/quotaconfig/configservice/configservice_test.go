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

package configservice

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/impl/memory"

	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/cachingtest"
	pb "go.chromium.org/luci/server/quotabeta/proto"
	"go.chromium.org/luci/server/quotabeta/quotaconfig"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestConfigService(t *testing.T) {
	t.Parallel()

	Convey("ConfigService", t, func() {
		ctx := cachingtest.WithGlobalCache(context.Background(), map[string]caching.BlobCache{
			"quota.configservice": cachingtest.NewBlobCache(),
		})

		Convey("New", func() {
			Convey("no cache", func() {
				shouldPanic := func() {
					New(context.Background(), "services/test", "policies.cfg")
				}
				So(shouldPanic, ShouldPanicLike, "no global cache available")
			})

			Convey("ok", func() {
				So(New(ctx, "services/test", "policies.cfg"), ShouldNotBeNil)
			})
		})

		Convey("Get", func() {
			c := &configService{
				cache:  caching.GlobalCache(ctx, "quota.configservice"),
				cfgSet: "services/test",
				path:   "policies.cfg",
			}
			So(c.cache, ShouldNotBeNil)
			bytes, err := proto.Marshal(&pb.Policy{
				Name:          "name",
				Resources:     1,
				Replenishment: 1,
			})
			So(err, ShouldBeNil)
			So(c.cache.Set(ctx, "name", bytes, 0), ShouldBeNil)

			Convey("policy not found", func() {
				p, err := c.Get(ctx, "missing")
				So(err, ShouldEqual, quotaconfig.ErrNotFound)
				So(p, ShouldBeNil)
			})

			Convey("found", func() {
				p, err := c.Get(ctx, "name")
				So(err, ShouldBeNil)
				So(p, ShouldResembleProto, &pb.Policy{
					Name:          "name",
					Resources:     1,
					Replenishment: 1,
				})
			})

			Convey("immutable", func() {
				p, err := c.Get(ctx, "name")
				So(err, ShouldBeNil)
				So(p, ShouldResembleProto, &pb.Policy{
					Name:          "name",
					Resources:     1,
					Replenishment: 1,
				})

				p.Resources++

				p, err = c.Get(ctx, "name")
				So(err, ShouldBeNil)
				So(p, ShouldResembleProto, &pb.Policy{
					Name:          "name",
					Resources:     1,
					Replenishment: 1,
				})
			})
		})

		Convey("Refresh", func() {
			c := &configService{
				cache:  caching.GlobalCache(ctx, "quota.configservice"),
				cfgSet: "services/test",
				path:   "policies.cfg",
			}
			So(c.cache, ShouldNotBeNil)

			Convey("not found", func() {
				ctx := cfgclient.Use(ctx, memory.New(map[config.Set]memory.Files{}))
				So(c.Refresh(ctx), ShouldErrLike, "no such config")
			})

			Convey("empty", func() {
				ctx := cfgclient.Use(ctx, memory.New(map[config.Set]memory.Files{
					"services/test": map[string]string{
						"policies.cfg": "",
					},
				}))
				So(c.Refresh(ctx), ShouldBeNil)
			})

			Convey("invalid config", func() {
				ctx := cfgclient.Use(ctx, memory.New(map[config.Set]memory.Files{
					"services/test": map[string]string{
						"policies.cfg": "invalid",
					},
				}))
				So(c.Refresh(ctx), ShouldErrLike, "unknown field name")
			})

			Convey("invalid policy", func() {
				ctx := cfgclient.Use(ctx, memory.New(map[config.Set]memory.Files{
					"services/test": map[string]string{
						"policies.cfg": `
							policy {
								name: "name",
								resources: -1,
							}
						`,
					},
				}))
				So(c.Refresh(ctx), ShouldErrLike, "did not pass validation")

				p, err := c.Get(ctx, "name")
				So(err, ShouldEqual, quotaconfig.ErrNotFound)
				So(p, ShouldBeNil)
			})

			Convey("one", func() {
				ctx := cfgclient.Use(ctx, memory.New(map[config.Set]memory.Files{
					"services/test": map[string]string{
						"policies.cfg": `
							policy {
								name: "name",
								resources: 2,
								replenishment: 1,
							}
						`,
					},
				}))
				So(c.Refresh(ctx), ShouldBeNil)

				p, err := c.Get(ctx, "name")
				So(err, ShouldBeNil)
				So(p, ShouldResembleProto, &pb.Policy{
					Name:          "name",
					Resources:     2,
					Replenishment: 1,
				})
			})

			Convey("many", func() {
				ctx := cfgclient.Use(ctx, memory.New(map[config.Set]memory.Files{
					"services/test": map[string]string{
						"policies.cfg": `
							policy {
								name: "policy1",
								resources: 1,
							}
							policy {
								name: "policy2",
							}
							policy {
								name: "policy3",
								replenishment: 1,
							}
						`,
					},
				}))
				So(c.Refresh(ctx), ShouldBeNil)

				p, err := c.Get(ctx, "policy1")
				So(err, ShouldBeNil)
				So(p, ShouldResembleProto, &pb.Policy{
					Name:      "policy1",
					Resources: 1,
				})

				p, err = c.Get(ctx, "policy2")
				So(err, ShouldBeNil)
				So(p, ShouldResembleProto, &pb.Policy{
					Name: "policy2",
				})

				p, err = c.Get(ctx, "policy3")
				So(err, ShouldBeNil)
				So(p, ShouldResembleProto, &pb.Policy{
					Name:          "policy3",
					Replenishment: 1,
				})
			})

			Convey("update", func() {
				bytes, err := proto.Marshal(&pb.Policy{
					Name:          "name",
					Resources:     1,
					Replenishment: 1,
				})
				So(err, ShouldBeNil)
				So(c.cache.Set(ctx, "name", bytes, 0), ShouldBeNil)

				p, err := c.Get(ctx, "name")
				So(err, ShouldBeNil)
				So(p, ShouldResembleProto, &pb.Policy{
					Name:          "name",
					Resources:     1,
					Replenishment: 1,
				})

				ctx := cfgclient.Use(ctx, memory.New(map[config.Set]memory.Files{
					"services/test": map[string]string{
						"policies.cfg": `
							policy {
								name: "name",
								resources: 2,
								replenishment: 1,
							}
						`,
					},
				}))
				So(c.Refresh(ctx), ShouldBeNil)

				p, err = c.Get(ctx, "name")
				So(err, ShouldBeNil)
				So(p, ShouldResembleProto, &pb.Policy{
					Name:          "name",
					Resources:     2,
					Replenishment: 1,
				})

				ctx = cfgclient.Use(ctx, memory.New(map[config.Set]memory.Files{
					"services/test": map[string]string{
						"policies.cfg": `
							policy {
								name: "name",
								resources: 2,
								replenishment: 2,
							}
						`,
					},
				}))
				So(c.Refresh(ctx), ShouldBeNil)

				p, err = c.Get(ctx, "name")
				So(err, ShouldBeNil)
				So(p, ShouldResembleProto, &pb.Policy{
					Name:          "name",
					Resources:     2,
					Replenishment: 2,
				})
			})
		})
	})
}
