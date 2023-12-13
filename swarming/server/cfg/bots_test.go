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

	Convey("Works", t, func() {
		bg, err := newBotGroups(goodBotsCfg)
		So(err, ShouldBeNil)
		So(bg.trustedDimensions, ShouldResemble, []string{"a", "b", "pool"})

		group0 := bg.directMatches["bot-0"]
		So(group0, ShouldNotBeNil)
		So(group0.Dimensions, ShouldResemble, map[string][]string{
			"pool": {"a"},
			"dim":  {"1", "2"},
		})

		group1 := bg.directMatches["bot-1"]
		So(group1, ShouldNotBeNil)
		So(group1.Dimensions, ShouldResemble, map[string][]string{
			"pool": {"b"},
		})

		So(bg.directMatches, ShouldResemble, map[string]*BotGroup{
			"bot-0":         group0,
			"vm0-m4":        group0,
			"vm1-m4":        group0,
			"vm2-m4":        group0,
			"gce-vms-0-111": group0,
			"bot-1":         group1,
			"vm0-m5":        group1,
			"vm1-m5":        group1,
			"vm2-m5":        group1,
		})

		prefixes := map[string]*BotGroup{}
		bg.prefixMatches.Walk(func(pfx string, val any) bool {
			prefixes[pfx] = val.(*BotGroup)
			return false
		})
		So(prefixes, ShouldResemble, map[string]*BotGroup{
			"gce-vms-0-": group0,
			"gce-vms-1-": group1,
		})

		So(bg.defaultGroup.Dimensions, ShouldResemble, map[string][]string{
			"pool":  {"unassigned"},
			"extra": {"1"},
		})
	})
}

func TestHostBotID(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		So(HostBotID("abc"), ShouldEqual, "abc")
		So(HostBotID("abc-def"), ShouldEqual, "abc-def")
		So(HostBotID("abc--def"), ShouldEqual, "abc")
		So(HostBotID("abc--def--xyz"), ShouldEqual, "abc")
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

	Convey("Empty", t, func() {
		So(call(&configpb.BotsCfg{
			TrustedDimensions: []string{"pool"},
		}), ShouldBeNil)
	})

	Convey("Good", t, func() {
		So(call(goodBotsCfg), ShouldBeNil)
	})

	Convey("Errors", t, func() {
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
			So(call(cs.cfg), ShouldResemble, []string{`in "bots.cfg" ` + cs.err})
		}
	})
}
