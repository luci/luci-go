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
			LogsCloudProject:     "logs-project",
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
			BotId: []string{"bot-2"},
			Auth: []*configpb.BotAuth{
				{RequireServiceAccount: []string{"some@example.com"}},
			},
			SystemServiceAccount: "bot",
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
		bg, err := newBotGroups(goodBotsCfg, map[string]string{
			"script.py": "some-script",
		})
		assert.That(t, err, should.ErrLike(nil))
		assert.That(t, bg.trustedDimensions, should.Match([]string{"a", "b", "pool"}))

		group0 := bg.directMatches["bot-0"]
		assert.That(t, group0.Dimensions, should.Match(map[string][]string{
			"pool": {"a"},
			"dim":  {"1", "2"},
		}))
		assert.That(t, group0.BotConfigScriptName, should.Equal("script.py"))
		assert.That(t, group0.BotConfigScriptBody, should.Equal("some-script"))
		assert.That(t, group0.BotConfigScriptSHA256, should.Equal("8edd90a7de22b7ad9af01fa01e3eca0da6e0a112359f6405a8a42b72df067565"))
		assert.That(t, group0.LogsCloudProject, should.Equal("logs-project"))

		group1 := bg.directMatches["bot-1"]
		assert.That(t, group1.Dimensions, should.Match(map[string][]string{
			"pool": {"b"},
		}))

		group2 := bg.directMatches["bot-2"]
		assert.That(t, group2.SystemServiceAccount, should.Equal("bot"))

		assert.That(t, bg.directMatches, should.Match(map[string]*BotGroup{
			"bot-0":         group0,
			"vm0-m4":        group0,
			"vm1-m4":        group0,
			"vm2-m4":        group0,
			"gce-vms-0-111": group0,
			"bot-1":         group1,
			"vm0-m5":        group1,
			"vm1-m5":        group1,
			"vm2-m5":        group1,
			"bot-2":         group2,
		}))

		prefixes := map[string]*BotGroup{}
		bg.prefixMatches.Walk(func(pfx string, val any) bool {
			prefixes[pfx] = val.(*BotGroup)
			return false
		})
		assert.That(t, prefixes, should.Match(map[string]*BotGroup{
			"gce-vms-0-": group0,
			"gce-vms-1-": group1,
		}))

		assert.That(t, bg.defaultGroup.Dimensions, should.Match(map[string][]string{
			"pool":  {"unassigned"},
			"extra": {"1"},
		}))
	})
}

func TestHostBotID(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		assert.That(t, HostBotID("abc"), should.Equal("abc"))
		assert.That(t, HostBotID("abc-def"), should.Equal("abc-def"))
		assert.That(t, HostBotID("abc--def"), should.Equal("abc"))
		assert.That(t, HostBotID("abc--def--xyz"), should.Equal("abc"))
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
			{
				cfg: groups(&configpb.BotGroup{
					Auth: []*configpb.BotAuth{
						{
							RequireLuciMachineToken: true,
							RequireServiceAccount:   []string{"some@example.com"},
							RequireGceVmToken:       &configpb.BotAuth_GCE{Project: "some-project"},
						},
					},
				}),
				err: `(bot_group #0 / auth #0): require_luci_machine_token and require_service_account and require_gce_vm_token can't be used at the same time`,
			},
			{
				cfg: groups(&configpb.BotGroup{
					Auth: []*configpb.BotAuth{
						{},
					},
				}),
				err: `(bot_group #0 / auth #0): if all auth requirements are unset, ip_whitelist must be set`,
			},
			{
				cfg: groups(&configpb.BotGroup{
					Auth: []*configpb.BotAuth{
						{RequireServiceAccount: []string{"not-email"}},
					},
				}),
				err: `(bot_group #0 / auth #0): bad service account email "not-email"`,
			},
			{
				cfg: groups(&configpb.BotGroup{
					Auth: []*configpb.BotAuth{
						{RequireGceVmToken: &configpb.BotAuth_GCE{}},
					},
				}),
				err: `(bot_group #0 / auth #0): missing project in require_gce_vm_token`,
			},
			{
				cfg: groups(&configpb.BotGroup{
					SystemServiceAccount: "zzz",
					Auth:                 defaultAuth,
				}),
				err: `(bot_group #0 / system_service_account): bad system_service_account email "zzz"`,
			},
			{
				cfg: groups(&configpb.BotGroup{
					SystemServiceAccount: "bot",
					Auth:                 defaultAuth,
				}),
				err: `(bot_group #0 / system_service_account): system_service_account "bot" requires auth.require_service_account to be used`,
			},
			{
				cfg: groups(&configpb.BotGroup{
					BotConfigScript: "not-python.xyz",
					Auth:            defaultAuth,
				}),
				err: `(bot_group #0 / bot_config_script ("not-python.xyz")): invalid bot_config_script: must end with .py`,
			},
			{
				cfg: groups(&configpb.BotGroup{
					BotConfigScript: "some/path.py",
					Auth:            defaultAuth,
				}),
				err: `(bot_group #0 / bot_config_script ("some/path.py")): invalid bot_config_script: must be a filename, not a path`,
			},
		}
		for _, cs := range testCases {
			assert.That(t, call(cs.cfg), should.Match([]string{`in "bots.cfg" ` + cs.err}))
		}
	})
}
