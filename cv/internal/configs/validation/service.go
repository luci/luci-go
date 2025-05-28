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

package validation

import (
	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/configs/srvcfg"
	listenerpb "go.chromium.org/luci/cv/settings/listener"
)

// validateListenerSettings validates a listener-settings file.
func validateListenerSettings(ctx *validation.Context, configSet, path string, content []byte) error {
	ctx.SetFile(path)
	cfg := listenerpb.Settings{}
	if err := prototext.Unmarshal(content, &cfg); err != nil {
		ctx.Error(err)
		return nil
	}
	if err := cfg.Validate(); err != nil {
		// TODO(crbug.com/1369189): enter context for the proto field path.
		ctx.Error(err)
		return nil
	}
	subscribedHosts := stringset.New(0)
	for i, sub := range cfg.GetGerritSubscriptions() {
		ctx.Enter("gerrit_subscriptions #%d", i+1)
		host := sub.GetHost()
		if !subscribedHosts.Add(host) {
			ctx.Errorf("subscription already exists for host %q", host)
		}
		if sub.GetMessageFormat() == listenerpb.Settings_GerritSubscription_MESSAGE_FORMAT_UNSPECIFIED {
			ctx.Enter("message_format")
			ctx.Errorf("must be specified")
			ctx.Exit()
		}
		ctx.Exit()
	}
	validateRegexp(ctx, "disabled_project_regexps", cfg.GetDisabledProjectRegexps())

	// Skip checking subscription configs on error.
	// The error must be due to an invalid regex in `disabled_project_regexps`.
	if isListenerEnabled, err := srvcfg.MakeListenerProjectChecker(&cfg); err == nil {
		watchedHostsByPrj, err := prjcfg.GetAllGerritHosts(ctx.Context)
		if err != nil {
			return transient.Tag.Apply(errors.Fmt("GetAllGerritHosts: %w", err))
		}
		for prj, hosts := range watchedHostsByPrj {
			// Unless it's matched with one of disabled_project_regexps,
			// all the Gerrit hosts must have a subscription config.
			if !isListenerEnabled(prj) {
				continue
			}

			ctx.Enter("project config %q", prj)
			for h := range hosts {
				if !subscribedHosts.Has(h) {
					ctx.Errorf("watches %q, but there is no gerrit_subscriptions for it", h)
				}
			}
			ctx.Exit()
		}
	}
	return nil
}
