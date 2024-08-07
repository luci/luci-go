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

var goodBotsCfg = &configpb.BotsCfg{
	TrustedDimensions: []string{"pool", "a", "b", "a"},
	BotGroup: []*configpb.BotGroup{
		{
			BotId:       []string{"bot-0", "vm{0..2}-m4", "gce-vms-0-111"},
			BotIdPrefix: []string{"gce-vms-0-"},
			Owners:      []string{"owner@example.com"},
			Auth: []*configpb.BotAuth{
				{RequireLuciMachineToken: true},
			},
			Dimensions: []string{
				"pool:a",
				"dim:2",
				"dim:1",
				"dim:1",
			},
			BotConfigScript:      "script.py",
			SystemServiceAccount: "system@accounts.example.com",
		},
		{
			BotId:       []string{"bot-1", "vm{0..2}-m5"},
			BotIdPrefix: []string{"gce-vms-1-"},
			Owners:      []string{"owner@example.com"},
			Auth: []*configpb.BotAuth{
				{RequireLuciMachineToken: true},
			},
			Dimensions: []string{
				"pool:b",
			},
			BotConfigScript:      "script.py",
			SystemServiceAccount: "system@accounts.example.com",
		},
		{
			Dimensions: []string{"pool:unassigned", "extra:1"},
			Auth: []*configpb.BotAuth{
				{
					RequireGceVmToken: &configpb.BotAuth_GCE{
						Project: "some-project",
					},
				},
				{RequireLuciMachineToken: true},
			},
		},
	},
}

func TestNewBotGroups(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		bg, err := newBotGroups(goodBotsCfg)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, bg.trustedDimensions, should.Resemble([]string{"a", "b", "pool"}))

		group0 := bg.directMatches["bot-0"]
		assert.Loosely(t, group0, should.NotBeNil)
		assert.Loosely(t, group0.Dimensions, should.Resemble(map[string][]string{
			"pool": {"a"},
			"dim":  {"1", "2"},
		}))

		group1 := bg.directMatches["bot-1"]
		assert.Loosely(t, group1, should.NotBeNil)
		assert.Loosely(t, group1.Dimensions, should.Resemble(map[string][]string{
			"pool": {"b"},
		}))

		assert.Loosely(t, bg.directMatches, should.Resemble(map[string]*BotGroup{
			"bot-0":         group0,
			"vm0-m4":        group0,
			"vm1-m4":        group0,
			"vm2-m4":        group0,
			"gce-vms-0-111": group0,
			"bot-1":         group1,
			"vm0-m5":        group1,
			"vm1-m5":        group1,
			"vm2-m5":        group1,
		}))

		prefixes := map[string]*BotGroup{}
		bg.prefixMatches.Walk(func(pfx string, val any) bool {
			prefixes[pfx] = val.(*BotGroup)
			return false
		})
		assert.Loosely(t, prefixes, should.Resemble(map[string]*BotGroup{
			"gce-vms-0-": group0,
			"gce-vms-1-": group1,
		}))

		assert.Loosely(t, bg.defaultGroup.Dimensions, should.Resemble(map[string][]string{
			"pool":  {"unassigned"},
			"extra": {"1"},
		}))
	})
}

func TestHostBotID(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		assert.Loosely(t, HostBotID("abc"), should.Equal("abc"))
		assert.Loosely(t, HostBotID("abc-def"), should.Equal("abc-def"))
		assert.Loosely(t, HostBotID("abc--def"), should.Equal("abc"))
		assert.Loosely(t, HostBotID("abc--def--xyz"), should.Equal("abc"))
	})
}

func TestValidateBotsCfg(t *testing.T) {
	t.Parallel()

	call := func(cfg *configpb.BotsCfg) []string {
		ctx := validation.Context{Context: context.Background()}
		ctx.SetFile("bots.cfg")
		validateBotsCfg(&ctx, cfg)
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
		assert.Loosely(t, call(&configpb.BotsCfg{
			TrustedDimensions: []string{"pool"},
		}), should.BeNil)
	})

	ftt.Run("Good", t, func(t *ftt.Test) {
		assert.Loosely(t, call(goodBotsCfg), should.BeNil)
	})

	ftt.Run("Errors", t, func(t *ftt.Test) {
		groups := func(gr ...*configpb.BotGroup) *configpb.BotsCfg {
			return &configpb.BotsCfg{
				TrustedDimensions: []string{"pool"},
				BotGroup:          gr,
			}
		}

		defaultAuth := []*configpb.BotAuth{
			{RequireLuciMachineToken: true},
		}

		testCases := []struct {
			cfg *configpb.BotsCfg
			err string
		}{
			{
				cfg: &configpb.BotsCfg{
					TrustedDimensions: []string{"", "pool"},
				},
				err: "(trusted_dimensions #0 (\"\")): the key cannot be empty",
			},
			{
				cfg: groups(&configpb.BotGroup{
					BotId: []string{""},
					Auth:  defaultAuth,
				}),
				err: `(bot_group #0 / bot_id #0 ("")): empty bot_id is not allowed`,
			},
			{
				cfg: groups(&configpb.BotGroup{
					BotId: []string{"vm{x..y}"},
					Auth:  defaultAuth,
				}),
				err: `(bot_group #0 / bot_id #0 ("vm{x..y}")): bad bot_id expression: bad expression - expecting a number or "}", got "x"`,
			},
			{
				cfg: groups(
					&configpb.BotGroup{
						BotId: []string{"vm0-m1"},
						Auth:  defaultAuth,
					},
					&configpb.BotGroup{
						BotId: []string{"vm{0..5}-m1"},
						Auth:  defaultAuth,
					}),
				err: `(bot_group #1 / bot_id #0 ("vm{0..5}-m1")): bot_id "vm0-m1" was already mentioned in group #0`,
			},
			{
				cfg: groups(&configpb.BotGroup{
					BotIdPrefix: []string{""},
					Auth:        defaultAuth,
				}),
				err: `(bot_group #0 / bot_id_prefix #0 ("")): empty bot_id_prefix is not allowed`,
			},
			{
				cfg: groups(
					&configpb.BotGroup{
						BotIdPrefix: []string{"pfx-a"},
						Auth:        defaultAuth,
					},
					&configpb.BotGroup{
						BotIdPrefix: []string{"pfx-a"},
						Auth:        defaultAuth,
					}),
				err: `(bot_group #1 / bot_id_prefix #0 ("pfx-a")): bot_id_prefix "pfx-a" is already specified in group #0`,
			},
			{
				cfg: groups(
					&configpb.BotGroup{
						BotIdPrefix: []string{"pfx-a"},
						Auth:        defaultAuth,
					},
					&configpb.BotGroup{
						BotIdPrefix: []string{"pfx-a-b"},
						Auth:        defaultAuth,
					}),
				err: `(bot_group #1 / bot_id_prefix #0 ("pfx-a-b")): bot_id_prefix "pfx-a-b" starts with prefix "pfx-a", defined in group #0, making group assignment for bots with prefix "pfx-a-b" ambiguous`,
			},
			{
				cfg: groups(
					&configpb.BotGroup{
						BotIdPrefix: []string{"pfx-a-b"},
						Auth:        defaultAuth,
					},
					&configpb.BotGroup{
						BotIdPrefix: []string{"pfx-a"},
						Auth:        defaultAuth,
					}),
				err: `(bot_group #1 / bot_id_prefix #0 ("pfx-a")): bot_id_prefix "pfx-a" is a prefix of "pfx-a-b", defined in group #0, making group assignment for bots with prefix "pfx-a-b" ambiguous`,
			},
			{
				cfg: groups(
					&configpb.BotGroup{
						Auth: defaultAuth,
					},
					&configpb.BotGroup{
						Auth: defaultAuth,
					}),
				err: `(bot_group #1): group #0 is already set as default`,
			},
			{
				cfg: groups(&configpb.BotGroup{}),
				err: `(bot_group #0): an "auth" entry is required`,
			},
			{
				cfg: groups(&configpb.BotGroup{
					Dimensions: []string{"not-pair"},
					Auth:       defaultAuth,
				}),
				err: `(bot_group #0 / dimensions #0 ("not-pair")): not a "key:value" pair`,
			},
			{
				cfg: groups(&configpb.BotGroup{
					Dimensions: []string{":val"},
					Auth:       defaultAuth,
				}),
				err: `(bot_group #0 / dimensions #0 (":val")): bad dimension key "": the key cannot be empty`,
			},
			{
				cfg: groups(&configpb.BotGroup{
					Dimensions: []string{"key:"},
					Auth:       defaultAuth,
				}),
				err: `(bot_group #0 / dimensions #0 ("key:")): bad dimension value "": the value cannot be empty`,
			},
		}
		for _, cs := range testCases {
			assert.Loosely(t, call(cs.cfg), should.Resemble([]string{`in "bots.cfg" ` + cs.err}))
		}
	})
}
