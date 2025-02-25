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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/impl/memory"

	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/cachingtest"
	pb "go.chromium.org/luci/server/quotabeta/proto"
	"go.chromium.org/luci/server/quotabeta/quotaconfig"
)

func TestConfigService(t *testing.T) {
	t.Parallel()

	ftt.Run("ConfigService", t, func(t *ftt.Test) {
		ctx := cachingtest.WithGlobalCache(context.Background(), map[string]caching.BlobCache{
			"quota.configservice": cachingtest.NewBlobCache(),
		})

		t.Run("New", func(t *ftt.Test) {
			t.Run("no cache", func(t *ftt.Test) {
				shouldPanic := func() {
					New(context.Background(), "services/test", "policies.cfg")
				}
				assert.Loosely(t, shouldPanic, should.PanicLike("no global cache available"))
			})

			t.Run("ok", func(t *ftt.Test) {
				assert.Loosely(t, New(ctx, "services/test", "policies.cfg"), should.NotBeNil)
			})
		})

		t.Run("Get", func(t *ftt.Test) {
			c := &configService{
				cache:  caching.GlobalCache(ctx, "quota.configservice"),
				cfgSet: "services/test",
				path:   "policies.cfg",
			}
			assert.Loosely(t, c.cache, should.NotBeNil)
			bytes, err := proto.Marshal(&pb.Policy{
				Name:          "name",
				Resources:     1,
				Replenishment: 1,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, c.cache.Set(ctx, "name", bytes, 0), should.BeNil)

			t.Run("policy not found", func(t *ftt.Test) {
				p, err := c.Get(ctx, "missing")
				assert.Loosely(t, err, should.Equal(quotaconfig.ErrNotFound))
				assert.Loosely(t, p, should.BeNil)
			})

			t.Run("found", func(t *ftt.Test) {
				p, err := c.Get(ctx, "name")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, p, should.Match(&pb.Policy{
					Name:          "name",
					Resources:     1,
					Replenishment: 1,
				}))
			})

			t.Run("immutable", func(t *ftt.Test) {
				p, err := c.Get(ctx, "name")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, p, should.Match(&pb.Policy{
					Name:          "name",
					Resources:     1,
					Replenishment: 1,
				}))

				p.Resources++

				p, err = c.Get(ctx, "name")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, p, should.Match(&pb.Policy{
					Name:          "name",
					Resources:     1,
					Replenishment: 1,
				}))
			})
		})

		t.Run("Refresh", func(t *ftt.Test) {
			c := &configService{
				cache:  caching.GlobalCache(ctx, "quota.configservice"),
				cfgSet: "services/test",
				path:   "policies.cfg",
			}
			assert.Loosely(t, c.cache, should.NotBeNil)

			t.Run("not found", func(t *ftt.Test) {
				ctx := cfgclient.Use(ctx, memory.New(map[config.Set]memory.Files{}))
				assert.Loosely(t, c.Refresh(ctx), should.ErrLike("no such config"))
			})

			t.Run("empty", func(t *ftt.Test) {
				ctx := cfgclient.Use(ctx, memory.New(map[config.Set]memory.Files{
					"services/test": map[string]string{
						"policies.cfg": "",
					},
				}))
				assert.Loosely(t, c.Refresh(ctx), should.BeNil)
			})

			t.Run("invalid config", func(t *ftt.Test) {
				ctx := cfgclient.Use(ctx, memory.New(map[config.Set]memory.Files{
					"services/test": map[string]string{
						"policies.cfg": "invalid",
					},
				}))
				assert.Loosely(t, c.Refresh(ctx), should.ErrLike("unknown field name"))
			})

			t.Run("invalid policy", func(t *ftt.Test) {
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
				assert.Loosely(t, c.Refresh(ctx), should.ErrLike("did not pass validation"))

				p, err := c.Get(ctx, "name")
				assert.Loosely(t, err, should.Equal(quotaconfig.ErrNotFound))
				assert.Loosely(t, p, should.BeNil)
			})

			t.Run("one", func(t *ftt.Test) {
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
				assert.Loosely(t, c.Refresh(ctx), should.BeNil)

				p, err := c.Get(ctx, "name")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, p, should.Match(&pb.Policy{
					Name:          "name",
					Resources:     2,
					Replenishment: 1,
				}))
			})

			t.Run("many", func(t *ftt.Test) {
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
				assert.Loosely(t, c.Refresh(ctx), should.BeNil)

				p, err := c.Get(ctx, "policy1")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, p, should.Match(&pb.Policy{
					Name:      "policy1",
					Resources: 1,
				}))

				p, err = c.Get(ctx, "policy2")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, p, should.Match(&pb.Policy{
					Name: "policy2",
				}))

				p, err = c.Get(ctx, "policy3")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, p, should.Match(&pb.Policy{
					Name:          "policy3",
					Replenishment: 1,
				}))
			})

			t.Run("update", func(t *ftt.Test) {
				bytes, err := proto.Marshal(&pb.Policy{
					Name:          "name",
					Resources:     1,
					Replenishment: 1,
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, c.cache.Set(ctx, "name", bytes, 0), should.BeNil)

				p, err := c.Get(ctx, "name")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, p, should.Match(&pb.Policy{
					Name:          "name",
					Resources:     1,
					Replenishment: 1,
				}))

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
				assert.Loosely(t, c.Refresh(ctx), should.BeNil)

				p, err = c.Get(ctx, "name")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, p, should.Match(&pb.Policy{
					Name:          "name",
					Resources:     2,
					Replenishment: 1,
				}))

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
				assert.Loosely(t, c.Refresh(ctx), should.BeNil)

				p, err = c.Get(ctx, "name")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, p, should.Match(&pb.Policy{
					Name:          "name",
					Resources:     2,
					Replenishment: 2,
				}))
			})
		})
	})
}
