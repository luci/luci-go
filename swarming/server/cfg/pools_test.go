// Copyright 2023 The LUCI Authors.
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

package cfg

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"

	configpb "go.chromium.org/luci/swarming/proto/config"
)

var goodPoolsCfg = &configpb.PoolsCfg{
	Pool: []*configpb.Pool{
		{
			Name:             []string{"a"},
			Realm:            "test:1",
			DefaultTaskRealm: "test:default",
		},
		{
			Name:  []string{"b", "c"},
			Realm: "test:2",
		},
	},
}

func TestNewPoolsConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		poolsCfg, err := newPoolsConfig(goodPoolsCfg)
		assert.Loosely(t, err, should.BeNil)
		pools := poolsCfg.pools
		assert.Loosely(t, pools, should.HaveLength(3))
		assert.Loosely(t, pools["a"].Realm, should.Equal("test:1"))
		assert.Loosely(t, pools["b"].Realm, should.Equal("test:2"))
		assert.Loosely(t, pools["c"].Realm, should.Equal("test:2"))

		assert.Loosely(t, pools["a"].DefaultTaskRealm, should.Equal("test:default"))
		assert.Loosely(t, pools["b"].DefaultTaskRealm, should.BeEmpty)
		assert.Loosely(t, pools["c"].DefaultTaskRealm, should.BeEmpty)
	})
}

func TestPoolsValidation(t *testing.T) {
	t.Parallel()

	call := func(cfg *configpb.PoolsCfg) []string {
		ctx := validation.Context{Context: context.Background()}
		ctx.SetFile("pools.cfg")
		validatePoolsCfg(&ctx, cfg)
		if err := ctx.Finalize(); err != nil {
			var verr *validation.Error
			errors.As(err, &verr)
			out := make([]string, len(verr.Errors))
			for i, err := range verr.Errors {
				out[i] = err.Error()
			}
			return out
		}
		return nil
	}

	ftt.Run("Empty", t, func(t *ftt.Test) {
		assert.Loosely(t, call(&configpb.PoolsCfg{}), should.BeNil)
	})

	ftt.Run("Good", t, func(t *ftt.Test) {
		assert.Loosely(t, call(goodPoolsCfg), should.BeNil)
	})

	ftt.Run("Errors", t, func(t *ftt.Test) {
		onePool := func(p *configpb.Pool) *configpb.PoolsCfg {
			return &configpb.PoolsCfg{
				Pool: []*configpb.Pool{p},
			}
		}

		testCases := []struct {
			cfg *configpb.PoolsCfg
			err string
		}{
			{
				cfg: onePool(&configpb.Pool{
					Name:       []string{"a"},
					Realm:      "test:1",
					Schedulers: &configpb.Schedulers{},
				}),
				err: "(pool #1 (a)): setting deprecated field `schedulers`",
			},
			{
				cfg: onePool(&configpb.Pool{
					Name:          []string{"a"},
					Realm:         "test:1",
					BotMonitoring: "bzzz",
				}),
				err: "(pool #1 (a)): setting deprecated field `bot_monitoring`",
			},
			{
				cfg: onePool(&configpb.Pool{
					Realm: "test:1",
				}),
				err: "(pool #1 (unnamed)): at least one pool name must be given",
			},
			{
				cfg: onePool(&configpb.Pool{
					Name:  []string{"a", ""},
					Realm: "test:1",
				}),
				err: `(pool #1 (a,)): bad pool name "": the value cannot be empty`,
			},
			{
				cfg: onePool(&configpb.Pool{
					Name:  []string{"a", "a"},
					Realm: "test:1",
				}),
				err: "(pool #1 (a,a)): pool \"a\" was already declared",
			},
			{
				cfg: onePool(&configpb.Pool{
					Name: []string{"a"},
				}),
				err: "(pool #1 (a)): missing required `realm` field",
			},
			{
				cfg: onePool(&configpb.Pool{
					Name:  []string{"a"},
					Realm: "not-global",
				}),
				err: "(pool #1 (a)): bad `realm` field: bad global realm name \"not-global\" - should be <project>:<realm>",
			},
			{
				cfg: onePool(&configpb.Pool{
					Name:             []string{"a"},
					Realm:            "test:1",
					DefaultTaskRealm: "not-global",
				}),
				err: "(pool #1 (a)): bad `default_task_realm` field: bad global realm name \"not-global\" - should be <project>:<realm>",
			},
		}
		for _, cs := range testCases {
			assert.Loosely(t, call(cs.cfg), should.Resemble([]string{`in "pools.cfg" ` + cs.err}))
		}
	})
}
