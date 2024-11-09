// Copyright 2018 The LUCI Authors.
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

package memory

import (
	"context"
	"testing"

	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/gce/api/config/v1"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("Delete", t, func(t *ftt.Test) {
		c := context.Background()
		srv := &Config{}

		t.Run("nil", func(t *ftt.Test) {
			cfg, err := srv.Delete(c, nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg, should.Resemble(&emptypb.Empty{}))
		})

		t.Run("empty", func(t *ftt.Test) {
			cfg, err := srv.Delete(c, &config.DeleteRequest{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg, should.Resemble(&emptypb.Empty{}))
		})

		t.Run("ID", func(t *ftt.Test) {
			cfg, err := srv.Delete(c, &config.DeleteRequest{
				Id: "id",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg, should.Resemble(&emptypb.Empty{}))
		})

		t.Run("deleted", func(t *ftt.Test) {
			srv.cfg.Store("id", &config.Config{})
			cfg, err := srv.Delete(c, &config.DeleteRequest{
				Id: "id",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg, should.Resemble(&emptypb.Empty{}))
			_, ok := srv.cfg.Load("id")
			assert.Loosely(t, ok, should.BeFalse)
		})
	})

	ftt.Run("Ensure", t, func(t *ftt.Test) {
		c := context.Background()
		srv := &Config{}

		t.Run("nil", func(t *ftt.Test) {
			cfg, err := srv.Ensure(c, nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg, should.Resemble((*config.Config)(nil)))
			_, ok := srv.cfg.Load("")
			assert.Loosely(t, ok, should.BeTrue)
		})

		t.Run("empty", func(t *ftt.Test) {
			cfg, err := srv.Ensure(c, &config.EnsureRequest{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg, should.Resemble((*config.Config)(nil)))
			_, ok := srv.cfg.Load("")
			assert.Loosely(t, ok, should.BeTrue)
		})

		t.Run("ID", func(t *ftt.Test) {
			cfg, err := srv.Ensure(c, &config.EnsureRequest{
				Id: "id",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg, should.Resemble((*config.Config)(nil)))
			_, ok := srv.cfg.Load("id")
			assert.Loosely(t, ok, should.BeTrue)
		})

		t.Run("config", func(t *ftt.Test) {
			cfg, err := srv.Ensure(c, &config.EnsureRequest{
				Id: "id",
				Config: &config.Config{
					Prefix: "prefix",
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg, should.Resemble(&config.Config{
				Prefix: "prefix",
			}))
			_, ok := srv.cfg.Load("id")
			assert.Loosely(t, ok, should.BeTrue)
		})
	})

	ftt.Run("Get", t, func(t *ftt.Test) {
		c := context.Background()
		srv := &Config{}

		t.Run("not found", func(t *ftt.Test) {
			t.Run("nil", func(t *ftt.Test) {
				cfg, err := srv.Get(c, nil)
				assert.Loosely(t, err, should.ErrLike("no config found"))
				assert.Loosely(t, cfg, should.BeNil)
			})

			t.Run("empty", func(t *ftt.Test) {
				cfg, err := srv.Get(c, &config.GetRequest{})
				assert.Loosely(t, err, should.ErrLike("no config found"))
				assert.Loosely(t, cfg, should.BeNil)
			})

			t.Run("ID", func(t *ftt.Test) {
				cfg, err := srv.Get(c, &config.GetRequest{
					Id: "id",
				})
				assert.Loosely(t, err, should.ErrLike("no config found"))
				assert.Loosely(t, cfg, should.BeNil)
			})
		})

		t.Run("found", func(t *ftt.Test) {
			srv.cfg.Store("id", &config.Config{
				Prefix: "prefix",
			})
			cfg, err := srv.Get(c, &config.GetRequest{
				Id: "id",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg, should.Resemble(&config.Config{
				Prefix: "prefix",
			}))
		})
	})
}
