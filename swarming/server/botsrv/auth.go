// Copyright 2024 The LUCI Authors.
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

package botsrv

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/tokenserver/auth/machine"

	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/metrics"
)

// authenticateBot checks the bot credentials in the context match requirements
// from bots.cfg for this bot based on its ID.
//
// Returns no error if the bot is successfully authenticated, a fatal error
// if not, and a transient-tagged error if there was some unexpected error.
// A fatal error message will be returned to the caller as is and should not
// contain any internal details not already available to the caller.
//
// Logs errors inside.
func authenticateBot(ctx context.Context, botID string, methods []*configpb.BotAuth) error {
	if len(methods) == 0 {
		// This should not really be possible for validated configs.
		return errors.Reason("no authentication requirements in bots.cfg").Err()
	}

	// Checks if a single BotAuth requirement is satisfied.
	//
	// Additionally returns any lines to be logged if auth fails.
	checkAuth := func(ba *configpb.BotAuth) (logs []string, err error) {
		log := func(msg string, args ...any) {
			logs = append(logs, fmt.Sprintf(msg, args...))
		}

		// "hostid--deviceid" => "hostid", since the device name suffix is not
		// affecting authentication (see cfg.HostBotID for details).
		hostID := cfg.HostBotID(botID)

		// Fields for the monitoring metric with successful authentications.
		authMethodForMon := ""
		authConditionForMon := ""

		switch {
		case ba.RequireLuciMachineToken:
			tok := machine.GetMachineTokenInfo(ctx)
			if tok == nil {
				return logs, errors.Reason("no LUCI machine token in the request").Err()
			}
			// TODO(vadimsh): We should probably check tok.CA as well. This will require
			// updating configs to have it there in the first place.
			if tok.FQDN != hostID && !strings.HasPrefix(tok.FQDN, hostID+".") {
				log("Machine token CA: %d", tok.CA)
				log("Machine token SN: %s", hex.EncodeToString(tok.CertSN))
				return logs, errors.Reason("host ID %q doesn't match the LUCI token with FQDN %q", hostID, tok.FQDN).Err()
			}
			authMethodForMon, authConditionForMon = "luci_token", "-"

		case ba.RequireGceVmToken != nil:
			tok := openid.GetGoogleComputeTokenInfo(ctx)
			if tok == nil {
				return logs, errors.Reason("no GCE VM token in the request").Err()
			}
			switch {
			case tok.Instance != hostID:
				log("GCE token instance: %s", tok.Instance)
				log("GCE token project: %s", tok.Project)
				return logs, errors.Reason("expecting VM token from GCE instance %q, but got one from %q", hostID, tok.Instance).Err()
			case tok.Project != ba.RequireGceVmToken.Project:
				log("GCE token instance: %s", tok.Instance)
				log("GCE token project: %s", tok.Project)
				return logs, errors.Reason("%q is not an expected GCE project for host %q", tok.Project, hostID).Err()
			}
			authMethodForMon, authConditionForMon = "gce_vm_token", ba.RequireGceVmToken.Project

		case len(ba.RequireServiceAccount) != 0:
			ident := auth.GetState(ctx).PeerIdentity()
			if ident.Kind() != identity.User {
				return logs, errors.Reason("no OAuth or OpenID credentials in the request").Err()
			}
			email := ident.Value()
			if slices.Index(ba.RequireServiceAccount, email) == -1 {
				return logs, errors.Reason("the host %q is authenticated as %q which is not the expected service account for this host", hostID, email).Err()
			}
			authMethodForMon, authConditionForMon = "service_account", email

		default:
			// If no other method applies, the IP allowlist **must** be used. This is
			// validated when loading bots.cfg. Double check this.
			if ba.IpWhitelist == "" {
				return logs, errors.Reason("bad bot group config, no auth methods defined").Err()
			}
			authMethodForMon, authConditionForMon = "ip_allowlist", ba.IpWhitelist
		}

		// IP allowlist (if set) works in conjunction with other methods.
		if ba.IpWhitelist != "" {
			switch yes, err := auth.IsAllowedIP(ctx, ba.IpWhitelist); {
			case err != nil:
				return logs, errors.Annotate(err, "failed to check IP allowlist").Tag(transient.Tag).Err()
			case !yes:
				log("Bot IP %s is not in the allowlist %q", auth.GetState(ctx).PeerIP(), ba.IpWhitelist)
				return logs, errors.Reason("IP not allowed").Err()
			}
		}

		// Success! Report the metric.
		metrics.BotAuthSuccesses.Add(ctx, 1, authMethodForMon, authConditionForMon)

		return logs, nil
	}

	// Errors from all attempted auth methods.
	var authErrs []error
	// Logs to emit if all methods fail. Will be dropped if any method succeeds.
	var delayedLogs []string
	// True if already logged the bot ID.
	var loggedBotID bool
	// True if logged a failure already.
	var loggedFail bool

	// Check all methods one by one until first success or a transient error.
	for _, ba := range methods {
		switch details, err := checkAuth(ba); {
		case err == nil:
			if loggedFail {
				logging.Infof(ctx, "Another auth method succeeded %s", formatBotAuth(ba))
			}
			return nil
		case transient.Tag.In(err):
			logging.Errorf(ctx, "Bot ID: %s", botID)
			logging.Errorf(ctx, "Transient auth error %s: %s", formatBotAuth(ba), err)
			return err
		default:
			authErrs = append(authErrs, err)
			if ba.LogIfFailed {
				// Asked to log this error even if some other method succeeds.
				if !loggedBotID {
					logging.Errorf(ctx, "Bot ID: %s", botID)
					loggedBotID = true
				}
				logging.Errorf(ctx, "Preferred auth method %s: %s", formatBotAuth(ba), err)
				for _, msg := range details {
					logging.Errorf(ctx, "%s", msg)
				}
				loggedFail = true
			} else {
				// Delay logging until all methods fail. If at least one succeeds, these
				// logs are unimportant spam and will be dropped.
				delayedLogs = append(delayedLogs, fmt.Sprintf("Auth method %s: %s", formatBotAuth(ba), err))
				delayedLogs = append(delayedLogs, details...)
			}
		}
	}

	// All methods failed. Need logs from all of them for the investigation.
	if !loggedBotID {
		logging.Errorf(ctx, "Bot ID: %s", botID)
	}
	for _, msg := range delayedLogs {
		logging.Errorf(ctx, "%s", msg)
	}

	if len(authErrs) == 1 {
		return authErrs[0]
	}
	errs := make([]string, len(authErrs))
	for i, err := range authErrs {
		errs[i] = err.Error()
	}
	return errors.Reason("all auth methods failed: %s", strings.Join(errs, "; ")).Err()
}

// formatBotAuth formats BotAuth for logs.
func formatBotAuth(ba *configpb.BotAuth) string {
	blob, err := protojson.Marshal(ba)
	if err != nil {
		return fmt.Sprintf("<error: %s>", err)
	}
	// Reformat to get rid of protojson's non-determinism, since we compare logs
	// in tests.
	var buf bytes.Buffer
	if err := json.Compact(&buf, blob); err != nil {
		return fmt.Sprintf("<error: %s>", err)
	}
	return buf.String()
}
