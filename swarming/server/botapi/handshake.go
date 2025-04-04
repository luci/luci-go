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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"

	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/botinfo"
	"go.chromium.org/luci/swarming/server/botsession"
	"go.chromium.org/luci/swarming/server/botsrv"
	"go.chromium.org/luci/swarming/server/botstate"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/validate"
)

// HandshakeRequest is sent by the bot.
type HandshakeRequest struct {
	// Dimensions are the initial dimensions collected by the bot.
	//
	// They are initial in a sense that they are collected by the base version
	// of the bot code, without using custom hooks script yet. In many cases,
	// these dimensions are very different from what the bot would end up using
	// after loading the hooks. For that reason these dimensions aren't actually
	// written to the datastore (to avoid the bot "flapping" between two sets of
	// dimensions when it restart and performs the handshake). They are still
	// used to check if the bot should be quarantined, and may be used in debug
	// logs.
	//
	// At least `id` dimension must be set. It is the bot ID.
	Dimensions map[string][]string `json:"dimensions"`

	// State is (mostly) arbitrary JSON dict with various properties of the bot.
	//
	// Values here are not indexed and they do not affect how tasks are scheduled
	// on the bot. The server is still aware of some keys and checks them to
	// decide how to handle the bot calls.
	State botstate.Dict `json:"state,omitempty"`

	// Version is the bot's own version.
	//
	// It is a digest of the running bot archive. Here it is used FYI only.
	Version string `json:"version"`

	// SessionID is an ID of a new session the bot is opening.
	//
	// If the bot has already opened this session (i.e. this is a retry), this
	// session will be reused.
	//
	// May be absent in calls from very old bots. An auto-generated ID will be
	// used instead in that case. Such bots will be asked to self update on the
	// very first poll call.
	SessionID string `json:"session_id,omitempty"`
}

func (r *HandshakeRequest) ExtractSession() []byte   { return nil }
func (r *HandshakeRequest) ExtractDebugRequest() any { return r }

// HandshakeResponse is returned by the server.
type HandshakeResponse struct {
	// Session is a serialized bot session proto.
	Session []byte `json:"session"`
	// BotVersion is the bot version the bot should be running, as FYI to the bot.
	BotVersion string `json:"bot_version"`
	// ServerVersion is the current server version, as FYI to the bot.
	ServerVersion string `json:"server_version"`
	// BotConfig is the body of the custom hooks script, if any.
	BotConfig string `json:"bot_config,omitempty"`
	// BotConfigName is the name of the custom hooks script or "bot_config.py".
	BotConfigName string `json:"bot_config_name"`
	// BotConfigRev is the revision of the main bot_config.py (not the hooks script).
	BotConfigRev string `json:"bot_config_rev"`
	// BotGroupCfg is derived from the server's bots.cfg.
	BotGroupCfg BotGroupCfg `json:"bot_group_cfg"`
	// RBE defines how the bot should be interacting with RBE API.
	RBE *BotRBEParams `json:"rbe,omitempty"`
}

// BotGroupCfg is derived from the server's bots.cfg and sent to the bot.
type BotGroupCfg struct {
	// Dimensions is the server-assigned dimensions from bots.cfg.
	Dimensions map[string][]string `json:"dimensions"`
}

// BotRBEParams is sent to the bot by the handshake and poll handlers.
//
// Describes how the bot should be interacting with RBE API.
type BotRBEParams struct {
	// Instance if the full RBE instance name the bot should be using.
	Instance string `json:"instance"`
	// HybridMode, if true, indicates to use RBE and native scheduler together.
	HybridMode bool `json:"hybrid_mode"`
	// Sleep is a legacy unused field preserved for compatibility.
	Sleep float64 `json:"sleep"`
	// PollToken is a legacy unused field preserved for compatibility.
	PollToken string `json:"poll_token"`
}

// Handshake implements the bot handshake handler.
//
// It is the first ever request sent by the bot. It registers the bot in the
// datastore and establishes a new bot session.
func (srv *BotAPIServer) Handshake(ctx context.Context, body *HandshakeRequest, _ *botsrv.Request) (botsrv.Response, error) {
	// Always use the latest config (not cached) when the bot is reconnecting,
	// since otherwise the bot may "see" the config going back in time (if the
	// latest call before the restart saw a newer config already). For all
	// subsequent bot API calls we'll be using `last_seen_config` field in the
	// bot session to skip unnecessary config fetches. But here we don't have
	// the session yet and can't use this mechanism.
	conf, err := srv.cfg.Latest(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error loading service config: %s", err)
	}

	// Extract bot ID from dimensions to authorize the bot before doing anything
	// else. Dimensions will be fully validated later.
	botID, err := peekBotID(body.Dimensions)
	if err != nil {
		return nil, err
	}

	// Authorize the bot by checking it uses the expected credentials.
	botGroup := conf.BotGroup(botID)
	if err := srv.authorizeBot(ctx, botID, botGroup.Auth); err != nil {
		if transient.Tag.In(err) {
			return nil, status.Errorf(codes.Internal, "transient error checking bot credentials: %s", err)
		}
		return nil, status.Errorf(codes.Unauthenticated, "the bot is not in bots.cfg or wrong credentials passed: %s", err)
	}

	// Validate or auto-generate the session ID. Auto-generated IDs are used only
	// by very old bots that will self-update on the very first poll.
	sessionID := body.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("autogen-%d-%d", clock.Now(ctx).UnixMilli(), mathrand.Int63(ctx))
	}
	if err := validate.SessionID(sessionID); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad `session_id` %q: %s", sessionID, err)
	}

	// Validate dimensions. We'll quarantine the bot if they are malformed.
	var errs errors.MultiError
	for _, err := range validate.BotDimensions(body.Dimensions) {
		errs = append(errs, errors.Annotate(err, "bad dimensions").Err())
	}

	// Validate the state is a correct JSON. We'll quarantine the bot if it isn't.
	if err := body.State.Unseal(); err != nil {
		errs = append(errs, errors.Annotate(err, "bad state dict").Err())
		body.State = botstate.Dict{}
	}

	// Update the bot state by marking the bot as "in handshake now". This is used
	// by the monitoring cron to distinguish fully connected bots from bots that
	// have just appeared and may not have full set of dimensions yet. Put the
	// reported (but unused) dimensions into the state for debugging.
	body.State, err = botstate.Edit(body.State, func(state *botstate.EditableDict) error {
		if err := state.Write(botstate.HandshakingKey, true); err != nil {
			return errors.Annotate(err, "writing %s", botstate.HandshakingKey).Err()
		}
		if err := state.Write(botstate.InitialDimensionsKey, body.Dimensions); err != nil {
			return errors.Annotate(err, "writing %s", botstate.InitialDimensionsKey).Err()
		}
		return nil
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update the bot state dict: %s", err)
	}

	// Figure out if the bot should be using RBE.
	//
	// TODO: Start requiring pure RBE mode with instance set (just like in Poll
	// handler) once all bots are migrated to RBE. Right now Handshake is used
	// for pure-Swarming and hybrid bots as well.
	var rbeParams *BotRBEParams
	rbeConfig, err := conf.RBEConfig(botID)
	switch {
	case err != nil:
		errs = append(errs, errors.Annotate(err, "conflicting RBE config").Err())
	case rbeConfig.Instance != "":
		rbeParams = &BotRBEParams{
			Instance:   rbeConfig.Instance,
			HybridMode: rbeConfig.Mode == configpb.Pool_RBEMigration_BotModeAllocation_HYBRID,
			PollToken:  "legacy-unused-field",
		}
	}

	// Log all errors for easier debugging.
	for _, err := range errs {
		logging.Errorf(ctx, "Validation error: %s", err)
	}

	// See if the bot self-quarantined, in maintenance or sent invalid handshake
	// request. This also updates the quarantine message in the state if
	// necessary (e.g. there were validation errors). This will result in the bot
	// being placed in a quarantine, with these errors visible in the bot UI.
	var healthInfo botinfo.HealthInfo
	healthInfo, body.State, err = updateBotHealthInfo(body.State, body.Dimensions[botstate.QuarantinedKey], errs)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update the bot state dict: %s", err)
	}

	// Figure out what bot version the bot should be running.
	channel, botArchive := conf.BotChannel(botID)
	logging.Infof(ctx, "Bot channel: %s (%s)", channel, botArchive.Digest)

	// Get custom hooks.
	botConfig := botGroup.BotConfigScriptBody
	botConfigName := botGroup.BotConfigScriptName
	if botConfigName == "" {
		botConfigName = "bot_config.py"
	}

	// Initialize a new session. Note that we do not populate RBEEffectiveBotID
	// here since at this point we don't know it yet (the bot hasn't sent
	// all dimensions yet). We'll populate it in the very first /bot/poll that
	// happens soon after the handshake (after the bot collects all dimensions).
	session, err := botsession.Marshal(botsession.Create(botsession.SessionParameters{
		SessionID:    sessionID,
		BotID:        botID,
		BotGroup:     botGroup,
		RBEConfig:    rbeConfig,
		ServerConfig: conf,
		DebugInfo:    botsession.DebugInfo(ctx, srv.version),
		Now:          clock.Now(ctx),
	}), srv.hmacSecret)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "fail to marshal session proto: %s", err)
	}

	// Register the BotInfo and BotEvent. Note that BotEventConnected doesn't set
	// dimensions, since when it happens the bot doesn't have custom hooks yet and
	// it doesn't know its full set of dimensions (since they are generally
	// derived by custom hooks). Instead, if the bot is connecting for the first
	// time, only dimensions assigned to it via bots.cfg config (as supplied by
	// BotGroup field) will be stored in BotInfo. If the bot is reconnecting after
	// a restart, its dimensions won't be changed. To help in debugging issues,
	// these initial dimensions are placed into the state instead (see above).
	update := &botinfo.Update{
		BotID:              botID,
		BotGroupDimensions: botGroup.Dimensions,
		State:              &body.State,
		EventType:          model.BotEventConnected,
		EventDedupKey:      sessionID,
		TasksManager:       srv.tasksManager,
		CallInfo: botCallInfo(ctx, &botinfo.CallInfo{
			SessionID: sessionID,
			Version:   body.Version,
		}),
		HealthInfo: &healthInfo,
	}
	if err := srv.submitUpdate(ctx, update); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update bot info: %s", err)
	}

	return &HandshakeResponse{
		Session:       session,
		BotVersion:    botArchive.Digest,
		ServerVersion: srv.version,
		BotConfig:     botConfig,
		BotConfigName: botConfigName,
		BotConfigRev:  botArchive.BotConfigRev,
		BotGroupCfg: BotGroupCfg{
			Dimensions: botGroup.Dimensions,
		},
		RBE: rbeParams,
	}, nil
}
