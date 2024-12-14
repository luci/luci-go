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

// Package botsession implements marshaling of Bot Session protos.
package botsession

import (
	"context"
	"crypto/sha256"
	"fmt"
	"slices"
	"time"

	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"

	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/hmactoken"
)

// Expiry is how long a new Swarming session token will last.
const Expiry = time.Hour

// cryptoCtx is used whe signing and checking the token as a cryptographic
// context (to make sure produced token can't be incorrectly used in other
// protocols that use the same secret key).
const cryptoCtx = "swarming.Session"

// Marshal serializes and signs a bot session proto.
func Marshal(s *internalspb.Session, secret *hmactoken.Secret) ([]byte, error) {
	blob, err := proto.Marshal(s)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(&internalspb.SessionToken{
		Kind: &internalspb.SessionToken_HmacTagged_{
			HmacTagged: &internalspb.SessionToken_HmacTagged{
				Session:    blob,
				HmacSha256: secret.Tag([]byte(cryptoCtx), blob),
			},
		},
	})
}

// Unmarshal checks the signature and deserializes a bot session.
//
// Doesn't check expiration time or validity of any Session fields.
func Unmarshal(tok []byte, secret *hmactoken.Secret) (*internalspb.Session, error) {
	var wrap internalspb.SessionToken
	if err := proto.Unmarshal(tok, &wrap); err != nil {
		return nil, errors.Annotate(err, "unmarshaling SessionToken").Err()
	}

	var blob []byte
	switch val := wrap.Kind.(type) {
	case *internalspb.SessionToken_HmacTagged_:
		if !secret.Verify([]byte(cryptoCtx), val.HmacTagged.Session, val.HmacTagged.HmacSha256) {
			return nil, errors.Reason("bad session token MAC").Err()
		}
		blob = val.HmacTagged.Session
	default:
		return nil, errors.Reason("unsupported session token format").Err()
	}

	s := &internalspb.Session{}
	if err := proto.Unmarshal(blob, s); err != nil {
		return nil, errors.Annotate(err, "unmarshaling Session").Err()
	}
	return s, nil
}

// SessionParameters encapsulates arguments of Create function.
type SessionParameters struct {
	// SessionID is the new session's ID as reported by the bot.
	SessionID string
	// BotID is the bot ID as reported by the bot.
	BotID string
	// BotGroup is the matching bot group config as looked up from bots.cfg.
	BotGroup *cfg.BotGroup
	// RBEConfig is the matching bot RBE config as looked up from pools.cfg.
	RBEConfig cfg.RBEConfig
	// ServerConfig is the config instance used to look up BotGroup and RBEConfig.
	ServerConfig *cfg.Config
	// DebugInfo to put into the session proto.
	DebugInfo *internalspb.DebugInfo
	// Now is the current time to use to calculate the expiration timestamp.
	Now time.Time
}

// Create initializes a new Session proto for an authorized connecting bot.
//
// Assumes all parameters have been validated already.
func Create(params SessionParameters) *internalspb.Session {
	return &internalspb.Session{
		BotId:     params.BotID,
		SessionId: params.SessionID,
		Expiry:    timestamppb.New(params.Now.Add(Expiry)),
		DebugInfo: params.DebugInfo,
		BotConfig: &internalspb.BotConfig{
			// Use default expiry here as well in a new session. It will be updated
			// to a larger value before we launch a task to make sure the captured
			// config can survive as long as the task (but not much longer). This will
			// be needed to allow the task to complete even if the bot is removed from
			// the config.
			Expiry:               timestamppb.New(params.Now.Add(Expiry)),
			DebugInfo:            params.DebugInfo,
			BotAuth:              params.BotGroup.Auth,
			SystemServiceAccount: params.BotGroup.SystemServiceAccount,
			LogsCloudProject:     params.BotGroup.LogsCloudProject,
			RbeInstance:          params.RBEConfig.Instance,
		},
		HandshakeConfigHash: handshakeConfigHash(params.BotGroup),
		RbeBotSessionId:     "", // will be populate later when the bot opens RBE session
		LastSeenConfig:      timestamppb.New(params.ServerConfig.VersionInfo.Fetched),
	}
}

// handshakeConfigHash is a hash of bot config parameters that affect the
// bot session.
//
// The hash of these parameters is capture in /handshake handler and put into
// the session token. If a /poll handler notices the current hash doesn't match
// the hash in the session, it will ask the bot to restart to pick up new
// parameters.
//
// This function is tightly coupled to what /handshake returns to the bot and
// to how bot uses these values.
func handshakeConfigHash(cfg *cfg.BotGroup) []byte {
	var lines []string

	// Need to restart the bot whenever the injected hooks script changes.
	if cfg.BotConfigScriptSHA256 != "" {
		lines = append(lines, fmt.Sprintf("config_script_sha256:%s", cfg.BotConfigScriptSHA256))
	}

	// Hooks script name (e.g. `android.py`) is exposed as a `bot_config`
	// dimension that hooks (in particular the default bot_config.py) can
	// theoretically react to. Need to restart the bot if the hooks script name
	// changes. This is rare.
	if cfg.BotConfigScriptName != "" {
		lines = append(lines, fmt.Sprintf("config_script_name:%s", cfg.BotConfigScriptName))
	}

	// Need to restart the bot whenever its server-assigned dimensions change,
	// since bot hooks can examine them (in particular in startup hooks).
	for key, vals := range cfg.Dimensions {
		for _, val := range vals {
			lines = append(lines, fmt.Sprintf("dimension:%s:%s", key, val))
		}
	}

	// Hash all that data in a deterministic way.
	slices.Sort(lines)
	h := sha256.New()
	for i, l := range lines {
		if i != 0 {
			_, _ = h.Write([]byte{'\n'})
		}
		_, _ = h.Write([]byte(l))
	}
	return h.Sum(nil)
}

// FormatForDebug formats the session proto for the debug log.
func FormatForDebug(s *internalspb.Session) string {
	blob, err := prototext.MarshalOptions{
		Multiline: true,
		Indent:    "  ",
	}.Marshal(s)
	if err != nil {
		return fmt.Sprintf("<error: %s>", err)
	}
	return string(blob)
}

// DebugInfo generates new DebugInfo proto identifying the current request.
func DebugInfo(ctx context.Context, backendVer string) *internalspb.DebugInfo {
	return &internalspb.DebugInfo{
		Created:         timestamppb.New(clock.Now(ctx)),
		SwarmingVersion: backendVer,
		RequestId:       trace.SpanContextFromContext(ctx).TraceID().String(),
	}
}
