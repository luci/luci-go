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
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/armon/go-radix"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/data/text/intsetexpr"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/validation"

	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/validate"
)

// unassignedPools is returned as pools of a bot not in the config.
var unassignedPools = []string{"unassigned"}

// BotGroup is one parsed section of bots.cfg config.
//
// It defines configuration that applies to all bots within that section.
type BotGroup struct {
	// Dimensions is bot dimensions assigned to matched bots via the config.
	//
	// Includes as least "pool" dimension, but potentially more.
	Dimensions map[string][]string

	// Auth defines how to authenticate bot API calls.
	//
	// There's always at least one element (but can be more if multiple auth
	// methods are allowed).
	Auth []*configpb.BotAuth

	// SystemServiceAccount is what account to use on bots when authenticating
	// calls to various system-level services (like CAS and CIPD) required for
	// correct operation of the bot.
	//
	// Either an empty string, a literal string "bot" or a service account email.
	SystemServiceAccount string

	// BotConfigScriptName is an optional name of a custom hooks script.
	//
	// Its existence is validated when ingesting the config. If empty, the bot is
	// using only the default bot_config.py (embedded into the bot archive).
	BotConfigScriptName string

	// BotConfigScriptBody is the body of the custom hooks script.
	//
	// It is an empty string if the bot is not using custom hooks script.
	BotConfigScriptBody string

	// BotConfigScriptSHA256 is a SHA256 hex digest of the custom hooks script.
	//
	// It is an empty string if the bot is not using custom hooks script.
	BotConfigScriptSHA256 string

	// LogsCloudProject is a Cloud Project name where the bot uploads its logs.
	//
	// This eventually shows up as part of bot info in the UI. Swarming itself
	// doesn't access this project.
	LogsCloudProject string
}

// Pools returns pools assigned to the bot or ["unassigned"] if not set.
//
// The returned slice always has at least one element.
func (gr *BotGroup) Pools() []string {
	if pools := gr.Dimensions["pool"]; len(pools) > 0 {
		return pools
	}
	return unassignedPools
}

// HostBotID takes a bot ID like `<host>--<sfx>` and returns just `<host>`.
//
// Bot IDs like `<host>--<sfx>` are called composite. They are used to represent
// multiple bots running on the same host (e.g. as docker containers) sharing
// the same host credentials. The `<host>` part identifies this host. It is used
// when checking the authentication tokens and looking up the bot group config.
//
// If the bot ID is not composite, returns it as is.
func HostBotID(botID string) string {
	if hostID, _, ok := strings.Cut(botID, "--"); ok {
		return hostID
	}
	return botID
}

// botGroups contains parsed bots.cfg config.
//
// See Config.BotGroup(...) for where it is queried.
type botGroups struct {
	trustedDimensions []string             // dimensions enforced by the server
	directMatches     map[string]*BotGroup // bot ID => its config
	prefixMatches     *radix.Tree          // bot ID prefix => config
	defaultGroup      *BotGroup            // the fallback, always non-nil
}

type configHooksScript struct {
	body   string
	sha256 string
}

// newBotGroups converts bots.cfg into a queryable representation.
//
// bots.cfg here already passed the validation when it was first ingested. It
// is possible the server code itself changed and the existing config is no
// longer correct in some bad way. An error is returned in that case.
func newBotGroups(cfg *configpb.BotsCfg, scripts map[string]string) (*botGroups, error) {
	bg := &botGroups{
		trustedDimensions: stringset.NewFromSlice(cfg.TrustedDimensions...).ToSortedSlice(),
		directMatches:     map[string]*BotGroup{},
		prefixMatches:     radix.New(),
		// This is the hardcoded default group that will be replaced by the default
		// group from the config, if there's any. A default group is designated in
		// the config by absence of bot_id and bot_id_prefix fields.
		defaultGroup: &BotGroup{
			Dimensions: map[string][]string{"pool": unassignedPools},
		},
	}

	scriptsBundle := make(map[string]configHooksScript, len(scripts))
	for name, body := range scripts {
		scriptsBundle[name] = configHooksScript{
			body:   body,
			sha256: sha256hex(body),
		}
	}

	for _, gr := range cfg.BotGroup {
		group, err := newBotGroup(gr, scriptsBundle)
		if err != nil {
			return nil, err
		}
		if len(gr.BotId) == 0 && len(gr.BotIdPrefix) == 0 {
			bg.defaultGroup = group
		} else {
			for _, botIDExpr := range gr.BotId {
				botIDs, err := intsetexpr.Expand(botIDExpr)
				if err != nil {
					return nil, errors.Annotate(err, "bad bot_id expression %q", botIDExpr).Err()
				}
				for _, botID := range botIDs {
					bg.directMatches[botID] = group
				}
			}
			for _, botPfx := range gr.BotIdPrefix {
				bg.prefixMatches.Insert(botPfx, group)
			}
		}
	}

	return bg, nil
}

// newBotGroup constructs BotGroup from its validated proto representation.
func newBotGroup(gr *configpb.BotGroup, scripts map[string]configHooksScript) (*BotGroup, error) {
	dims := map[string][]string{}
	for _, dim := range gr.Dimensions {
		key, val, ok := strings.Cut(dim, ":")
		if !ok {
			return nil, errors.Reason("invalid bot dimension %q", dim).Err()
		}
		dims[key] = append(dims[key], val)
	}
	for key, val := range dims {
		dims[key] = stringset.NewFromSlice(val...).ToSortedSlice()
	}

	// The script body must be present in properly validated configs. But if for
	// whatever reason it is missing, consistently disable it for the group (by
	// unsetting BotConfigScriptName).
	scriptName := gr.BotConfigScript
	scriptBody := ""
	scriptSHA256 := ""
	if scriptName != "" {
		if script, ok := scripts[scriptName]; ok {
			scriptBody = script.body
			scriptSHA256 = script.sha256
		} else {
			scriptName = ""
		}
	}

	return &BotGroup{
		Dimensions:            dims,
		Auth:                  gr.Auth,
		SystemServiceAccount:  gr.SystemServiceAccount,
		BotConfigScriptName:   scriptName,
		BotConfigScriptBody:   scriptBody,
		BotConfigScriptSHA256: scriptSHA256,
		LogsCloudProject:      gr.LogsCloudProject,
	}, nil
}

// sha256hex returns SHA256 digest of `b` as a hex string.
func sha256hex(b string) string {
	h := sha256.New()
	_, _ = h.Write([]byte(b))
	return hex.EncodeToString(h.Sum(nil))
}

// validateBotsCfg validates bots.cfg, writing errors into `ctx`.
func validateBotsCfg(ctx *validation.Context, cfg *configpb.BotsCfg) {
	seenPool := false
	for i, dim := range cfg.TrustedDimensions {
		seenPool = seenPool || dim == "pool"
		ctx.Enter("trusted_dimensions #%d (%q)", i, dim)
		if err := validate.DimensionKey(dim); err != nil {
			ctx.Errorf("%s", err)
		}
		ctx.Exit()
	}
	if !seenPool {
		ctx.Enter("trusted_dimensions")
		ctx.Errorf(`"pool" must be specified as a trusted dimension`)
		ctx.Exit()
	}

	// Explicitly mentioned bot_id => index of a group where it was mentioned.
	botIDs := map[string]int{}
	// bot_id_prefix => index of a group where it was defined.
	botIDPrefixes := map[string]int{}
	// Index of a group to use as a default fallback (there can be only one).
	defaultGroupIdx := -1

	// Validates bot_id value in a group and updates botIDs.
	validateGroupBotID := func(botIDExpr string, idx int) {
		if botIDExpr == "" {
			ctx.Errorf("empty bot_id is not allowed")
			return
		}
		ids, err := intsetexpr.Expand(botIDExpr)
		if err != nil {
			ctx.Errorf("bad bot_id expression: %s", err)
			return
		}
		for _, botID := range ids {
			if groupIdx, yes := botIDs[botID]; yes {
				ctx.Errorf("bot_id %q was already mentioned in group #%d", botID, groupIdx)
			} else {
				botIDs[botID] = idx
			}
		}
	}

	// Validates bot_id_prefixes and updates botIDPrefixes.
	validateGroupBotIDPrefix := func(botIDPfx string, idx int) {
		if botIDPfx == "" {
			ctx.Errorf("empty bot_id_prefix is not allowed")
			return
		}
		if groupIdx, yes := botIDPrefixes[botIDPfx]; yes {
			ctx.Errorf("bot_id_prefix %q is already specified in group #%d", botIDPfx, groupIdx)
			return
		}

		// There should be no "intersecting" prefixes, they introduce ambiguities.
		// This check is O(N^2) (considering validateGroupBotIDPrefix is called N
		// times and the loop below does N iterations), but it executes only when
		// the config is changing, so it is not a big deal.
		for knownPfx, groupIdx := range botIDPrefixes {
			if strings.HasPrefix(knownPfx, botIDPfx) {
				ctx.Errorf(
					"bot_id_prefix %q is a prefix of %q, defined in group #%d, "+
						"making group assignment for bots with prefix %q ambiguous",
					botIDPfx, knownPfx, groupIdx, knownPfx,
				)
			} else if strings.HasPrefix(botIDPfx, knownPfx) {
				ctx.Errorf(
					"bot_id_prefix %q starts with prefix %q, defined in group #%d, "+
						"making group assignment for bots with prefix %q ambiguous",
					botIDPfx, knownPfx, groupIdx, botIDPfx,
				)
			}
		}

		botIDPrefixes[botIDPfx] = idx
	}

	// Validates the string looks like an email.
	validateEmail := func(val, what string) {
		if _, err := identity.MakeIdentity("user:" + val); err != nil {
			ctx.Errorf("bad %s email %q", what, val)
		}
	}

	// Validates auth entry.
	validateAuth := func(cfg *configpb.BotAuth) {
		var fields []string
		if cfg.RequireLuciMachineToken {
			fields = append(fields, "require_luci_machine_token")
		}
		if len(cfg.RequireServiceAccount) != 0 {
			fields = append(fields, "require_service_account")
		}
		if cfg.RequireGceVmToken != nil {
			fields = append(fields, "require_gce_vm_token")
		}

		if len(fields) > 1 {
			ctx.Errorf("%s can't be used at the same time", strings.Join(fields, " and "))
		}
		if len(fields) == 0 && cfg.IpWhitelist == "" {
			ctx.Errorf("if all auth requirements are unset, ip_whitelist must be set")
		}

		for _, sa := range cfg.RequireServiceAccount {
			validateEmail(sa, "service account")
		}

		if cfg.RequireGceVmToken != nil && cfg.RequireGceVmToken.Project == "" {
			ctx.Errorf("missing project in require_gce_vm_token")
		}
	}

	// Validates system_service_account field.
	validateSystemServiceAccount := func(gr *configpb.BotGroup) {
		switch {
		case gr.SystemServiceAccount == "bot":
			// If it is 'bot', the bot auth must be configured to use OAuth, since we
			// need to get a bot token somewhere.
			for _, auth := range gr.Auth {
				if len(auth.RequireServiceAccount) != 0 {
					return // the config is good
				}
			}
			ctx.Errorf("system_service_account \"bot\" requires auth.require_service_account to be used")
		case gr.SystemServiceAccount != "":
			validateEmail(gr.SystemServiceAccount, "system_service_account")
		}
	}

	// Validates a "key:val" dimension string.
	validateFlatDimension := func(dim string) {
		key, val, ok := strings.Cut(dim, ":")
		if !ok {
			ctx.Errorf(`not a "key:value" pair`)
			return
		}
		if err := validate.DimensionKey(key); err != nil {
			ctx.Errorf("bad dimension key %q: %s", key, err)
		}
		if err := validate.DimensionValue(key, val); err != nil {
			ctx.Errorf("bad dimension value %q: %s", val, err)
		}
	}

	// Validates a path looks like a python file name.
	validateBotConfigScript := func(val string) {
		if !strings.HasSuffix(val, ".py") {
			ctx.Errorf("invalid bot_config_script: must end with .py")
		}
		if strings.ContainsAny(val, "\\/") {
			ctx.Errorf("invalid bot_config_script: must be a filename, not a path")
		}
	}

	for idx, gr := range cfg.BotGroup {
		ctx.Enter("bot_group #%d", idx)

		// Validate 'bot_id' field and make sure bot_id groups do not intersect.
		for i, botIDExpr := range gr.BotId {
			ctx.Enter("bot_id #%d (%q)", i, botIDExpr)
			validateGroupBotID(botIDExpr, idx)
			ctx.Exit()
		}

		// Validate 'bot_id_prefix' and make sure prefix groups do not intersect.
		for i, botIDPfx := range gr.BotIdPrefix {
			ctx.Enter("bot_id_prefix #%d (%q)", i, botIDPfx)
			validateGroupBotIDPrefix(botIDPfx, idx)
			ctx.Exit()
		}

		// A group without 'bot_id' and 'bot_id_prefix' is applied to bots that
		// don't fit any other groups. There should be at most one such group.
		if len(gr.BotId) == 0 && len(gr.BotIdPrefix) == 0 {
			if defaultGroupIdx != -1 {
				ctx.Errorf("group #%d is already set as default", defaultGroupIdx)
			} else {
				defaultGroupIdx = idx
			}
		}

		// Validate 'auth' and 'system_service_account' fields.
		if len(gr.Auth) == 0 {
			ctx.Errorf(`an "auth" entry is required`)
		} else {
			for i, auth := range gr.Auth {
				ctx.Enter("auth #%d", i)
				validateAuth(auth)
				ctx.Exit()
			}
		}
		if gr.SystemServiceAccount != "" {
			ctx.Enter("system_service_account")
			validateSystemServiceAccount(gr)
			ctx.Exit()
		}

		// Validate 'owners'. Just check they are emails.
		for i, entry := range gr.Owners {
			ctx.Enter("owners #%d (%q)", i, entry)
			validateEmail(entry, "owner")
			ctx.Exit()
		}

		// Validate 'dimensions'.
		for i, dim := range gr.Dimensions {
			ctx.Enter("dimensions #%d (%q)", i, dim)
			validateFlatDimension(dim)
			ctx.Exit()
		}

		// Validate 'bot_config_script' looks like a python file name.
		if gr.BotConfigScript != "" {
			ctx.Enter("bot_config_script (%q)", gr.BotConfigScript)
			validateBotConfigScript(gr.BotConfigScript)
			ctx.Exit()
		}

		// 'bot_config_script_content' must be unset. It is used internally by
		// Python code, we don't use it.
		if len(gr.BotConfigScriptContent) != 0 {
			ctx.Enter("bot_config_script_content")
			ctx.Errorf("this field is used only internally and must be unset in the config")
			ctx.Exit()
		}

		ctx.Exit()
	}
}
