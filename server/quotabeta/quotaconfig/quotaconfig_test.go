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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"

	pb "go.chromium.org/luci/server/quotabeta/proto"
)

func TestQuotaConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("Memory", t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run("Get", func(t *ftt.Test) {
			m := &Memory{
				policies: map[string]*pb.Policy{
					"name": {
						Name:          "name",
						Resources:     1,
						Replenishment: 1,
					},
				},
			}

			t.Run("policy not found", func(t *ftt.Test) {
				p, err := m.Get(ctx, "missing")
				assert.Loosely(t, err, should.Equal(ErrNotFound))
				assert.Loosely(t, p, should.BeNil)
			})

			t.Run("found", func(t *ftt.Test) {
				p, err := m.Get(ctx, "name")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, p, should.Match(&pb.Policy{
					Name:          "name",
					Resources:     1,
					Replenishment: 1,
				}))
			})

			t.Run("immutable", func(t *ftt.Test) {
				p, err := m.Get(ctx, "name")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, p, should.Match(&pb.Policy{
					Name:          "name",
					Resources:     1,
					Replenishment: 1,
				}))

				p.Resources++

				p, err = m.Get(ctx, "name")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, p, should.Match(&pb.Policy{
					Name:          "name",
					Resources:     1,
					Replenishment: 1,
				}))
			})
		})
	})

	ftt.Run("ValidatePolicy", t, func(t *ftt.Test) {
		ctx := &validation.Context{Context: context.Background()}

		t.Run("name", func(t *ftt.Test) {
			t.Run("nil", func(t *ftt.Test) {
				ValidatePolicy(ctx, nil)
				err := ctx.Finalize()
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.(*validation.Error).Errors, should.ErrLike("name must match"))
			})

			t.Run("empty", func(t *ftt.Test) {
				p := &pb.Policy{}

				ValidatePolicy(ctx, p)
				err := ctx.Finalize()
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.(*validation.Error).Errors, should.ErrLike("name must match"))
			})

			t.Run("invalid char", func(t *ftt.Test) {
				p := &pb.Policy{
					Name: "projects/project/users/${name}",
				}

				ValidatePolicy(ctx, p)
				err := ctx.Finalize()
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.(*validation.Error).Errors, should.ErrLike("name must match"))
			})

			t.Run("user", func(t *ftt.Test) {
				p := &pb.Policy{
					Name: "projects/project/users/${user}",
				}

				ValidatePolicy(ctx, p)
				assert.Loosely(t, ctx.Finalize(), should.BeNil)
			})

			t.Run("len", func(t *ftt.Test) {
				p := &pb.Policy{
					Name: "projects/project/users/user/policies/very-long-policy-name-exceeds-limit",
				}

				ValidatePolicy(ctx, p)
				err := ctx.Finalize()
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.(*validation.Error).Errors, should.ErrLike("name must not exceed"))
			})
		})

		t.Run("resources", func(t *ftt.Test) {
			t.Run("negative", func(t *ftt.Test) {
				p := &pb.Policy{
					Name:      "name",
					Resources: -1,
				}

				ValidatePolicy(ctx, p)
				err := ctx.Finalize()
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.(*validation.Error).Errors, should.ErrLike("resources must not be negative"))
			})

			t.Run("zero", func(t *ftt.Test) {
				p := &pb.Policy{
					Name: "name",
				}

				ValidatePolicy(ctx, p)
				assert.Loosely(t, ctx.Finalize(), should.BeNil)
			})

			t.Run("positive", func(t *ftt.Test) {
				p := &pb.Policy{
					Name:      "name",
					Resources: 1,
				}

				ValidatePolicy(ctx, p)
				assert.Loosely(t, ctx.Finalize(), should.BeNil)
			})
		})

		t.Run("replenishment", func(t *ftt.Test) {
			t.Run("negative", func(t *ftt.Test) {
				p := &pb.Policy{
					Name:          "name",
					Replenishment: -1,
				}

				ValidatePolicy(ctx, p)
				err := ctx.Finalize()

				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.(*validation.Error).Errors, should.ErrLike("replenishment must not be negative"))
			})

			t.Run("zero", func(t *ftt.Test) {
				p := &pb.Policy{
					Name: "name",
				}

				ValidatePolicy(ctx, p)
				assert.Loosely(t, ctx.Finalize(), should.BeNil)
			})
			t.Run("positive", func(t *ftt.Test) {
				p := &pb.Policy{
					Name:          "name",
					Replenishment: 1,
				}

				ValidatePolicy(ctx, p)
				assert.Loosely(t, ctx.Finalize(), should.BeNil)
			})
		})
	})
}
