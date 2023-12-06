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
	"go.chromium.org/luci/config/validation"

	configpb "go.chromium.org/luci/swarming/proto/config"

	. "github.com/smartystreets/goconvey/convey"
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

	Convey("Works", t, func() {
		pools, err := newPoolsConfig(goodPoolsCfg)
		So(err, ShouldBeNil)
		So(pools, ShouldHaveLength, 3)

		So(pools["a"].Realm, ShouldEqual, "test:1")
		So(pools["b"].Realm, ShouldEqual, "test:2")
		So(pools["c"].Realm, ShouldEqual, "test:2")

		So(pools["a"].DefaultTaskRealm, ShouldEqual, "test:default")
		So(pools["b"].DefaultTaskRealm, ShouldEqual, "")
		So(pools["c"].DefaultTaskRealm, ShouldEqual, "")
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

	Convey("Empty", t, func() {
		So(call(&configpb.PoolsCfg{}), ShouldBeNil)
	})

	Convey("Good", t, func() {
		So(call(goodPoolsCfg), ShouldBeNil)
	})

	Convey("Errors", t, func() {
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
			So(call(cs.cfg), ShouldResemble, []string{`in "pools.cfg" ` + cs.err})
		}
	})
}
