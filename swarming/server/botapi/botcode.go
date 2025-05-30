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

package botapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/tokenserver/auth/machine"

	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/model"
)

// BotCode serves the bot archive blob or an HTTP redirect to it.
//
// Used to bootstrap bots and by the self-updating bots.
//
// Uses optional "Version" route parameter. Its value is either a concrete bot
// archive digest to fetch, or an empty string (in which case the handler will
// serve a redirect to the current stable bot version).
//
// Attempts are made to utilize GAE's edge cache by setting the corresponding
// headers.
func (srv *BotAPIServer) BotCode(c *router.Context) {
	req := c.Request
	rw := c.Writer
	ctx := req.Context()
	version := c.Params.ByName("Version")

	// Check if we know the requested version.
	conf := srv.cfg.Cached(ctx)
	known := false
	if version != "" {
		known = version == conf.VersionInfo.StableBot.Digest || version == conf.VersionInfo.CanaryBot.Digest
		if !known {
			// Our config cache might be stale, check the latest one.
			var err error
			conf, err = srv.cfg.Latest(ctx)
			if err != nil {
				logging.Errorf(ctx, "Error fetching latest config: %s", err)
				http.Error(rw, "internal error fetching the config", http.StatusInternalServerError)
				return
			}
			known = version == conf.VersionInfo.StableBot.Digest || version == conf.VersionInfo.CanaryBot.Digest
		}
	}

	// If didn't ask for a concrete version, or asked for a version we don't
	// know, redirect to the latest stable version. Use a redirect, instead of
	// serving it directly, to utilize the GAE response cache.
	if version == "" || !known {
		if version != "" {
			logging.Warningf(ctx, "Requesting unknown version %s", version)
		}
		// TODO(b/362324087): Improve.
		switch err := srv.checkBotCodeAccess(ctx, req, conf); {
		case transient.Tag.In(err):
			logging.Errorf(ctx, "Error checking bot code access: %s", err)
			http.Error(rw, "internal error checking access", http.StatusInternalServerError)
		case err != nil:
			logging.Warningf(ctx, "Bot code access denied: %s", err)
			http.Error(rw, err.Error(), http.StatusForbidden)
		default:
			redirectToVersion(rw, req, conf.VersionInfo.StableBot.Digest)
		}
		return
	}

	// If the request has a query string, redirect to an URI without it, since
	// it is the one being cached by GAE.
	if len(req.URL.Query()) != 0 {
		logging.Infof(ctx, "Ignoring query string: %s", req.URL.Query().Encode())
		redirectToVersion(rw, req, version)
		return
	}

	// Get the bot code from the memory cache or fetch it from the datastore.
	blob, err := srv.botCodeCache.GetOrCreate(ctx, version, func() (blob []byte, exp time.Duration, err error) {
		var ba *cfg.BotArchiveInfo
		switch {
		case version == conf.VersionInfo.StableBot.Digest:
			ba = &conf.VersionInfo.StableBot
		case version == conf.VersionInfo.CanaryBot.Digest:
			ba = &conf.VersionInfo.CanaryBot
		default:
			panic("impossible, known == true")
		}
		blob, err = ba.FetchBotArchive(ctx)
		if err != nil {
			return nil, 0, err
		}
		return blob, time.Hour, nil
	})
	if err != nil {
		logging.Errorf(ctx, "Error fetching bot code: %s", err)
		http.Error(rw, "internal error fetching bot code", http.StatusInternalServerError)
		return
	}

	// Serve the bot code blob, asking GAE to cache it for a while.
	rw.Header().Set("Cache-Control", "public, max-age=3600")
	rw.Header().Set("Content-Type", "application/octet-stream")
	rw.Header().Set("Content-Disposition", `attachment; filename="swarming_bot.zip"`)
	_, _ = rw.Write(blob)
}

// checkBotCodeAccess checks if the caller has access to the bot code.
//
// Returns nil if yes, a fatal error if no, and a transient error if the check
// itself failed. A fatal error will be returned to the caller as is.
func (srv *BotAPIServer) checkBotCodeAccess(ctx context.Context, req *http.Request, conf *cfg.Config) error {
	// Check if using a bootstrap token.
	if tok := req.URL.Query().Get("tok"); tok != "" {
		who, err := model.ValidateBootstrapToken(ctx, tok)
		if err != nil {
			return err
		}
		logging.Infof(ctx, "Using a bootstrap token generated by %s", who)
		return nil
	}

	// Check if this is some known bot. Carry on with other checks if not.
	//
	// Bot ID can be supplied as:
	//   * bot_id query parameter.
	//   * X-Luci-Swarming-Bot-ID header.
	//   * As part of LUCI machine token.
	//   * As part of GCE VM identity token.
	botID := req.URL.Query().Get("bot_id")
	if botID == "" {
		botID = req.Header.Get("X-Luci-Swarming-Bot-ID")
	}
	if botID == "" {
		if tok := machine.GetMachineTokenInfo(ctx); tok != nil {
			botID, _, _ = strings.Cut(tok.FQDN, ".")
		}
	}
	if botID == "" {
		if tok := openid.GetGoogleComputeTokenInfo(ctx); tok != nil {
			botID = tok.Instance
		}
	}

	// Check the bot with this ID is using the expected credentials.
	if botID != "" {
		err := srv.authorizeBot(ctx, botID, conf.BotGroup(botID).Auth)
		if err == nil || transient.Tag.In(err) {
			return err
		}
	}

	// Check if the caller is allowed to bootstrap bots per the server ACL.
	res := acls.NewChecker(ctx, conf).CheckServerPerm(ctx, acls.PermPoolsCreateBot)
	if res.InternalError || res.Permitted {
		return res.ToTaggedError()
	}

	// As the final fallback, check if this is a known IP address by checking
	// "<server>-bots" IP allowlist. This method is deprecated. Log when it is
	// used.
	switch yes, err := auth.IsAllowedIP(ctx, fmt.Sprintf("%s-bots", srv.project)); {
	case err != nil:
		return transient.Tag.Apply(errors.Fmt("checking IP allowlist: %w", err))
	case yes:
		if botID == "" {
			botID = "<unknown>"
		}
		logging.Warningf(ctx, "Relying on deprecated IP allowlist check from bot %q and IP %s", botID, auth.GetState(ctx).PeerIP())
		return nil
	}

	// Use the error message from the ACL checking step, since it is the most
	// informative.
	return res.ToTaggedError()
}

// redirectToVersion serves an HTTP redirect to the corresponding bot code URL.
func redirectToVersion(rw http.ResponseWriter, req *http.Request, version string) {
	rw.Header()["Content-Type"] = nil // disable writing <html>, see http.Redirect
	http.Redirect(rw, req, "/swarming/api/v1/bot/bot_code/"+version, http.StatusFound)
}
