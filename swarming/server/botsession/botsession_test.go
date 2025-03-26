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

package botsession

import (
	"crypto/sha256"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/secrets"

	configpb "go.chromium.org/luci/swarming/proto/config"
	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/hmactoken"
)

func TestMarshaling(t *testing.T) {
	t.Parallel()

	secret := hmactoken.NewStaticSecret(secrets.Secret{
		Active: []byte("secret"),
	})

	session := &internalspb.Session{
		BotId:     "bot-id",
		SessionId: "session-id",
	}

	t.Run("Round trip", func(t *testing.T) {
		blob, err := Marshal(session, secret)
		assert.NoErr(t, err)

		restored, err := Unmarshal(blob, secret)
		assert.NoErr(t, err)

		assert.That(t, session, should.Match(restored))
	})

	t.Run("Bad MAC", func(t *testing.T) {
		blob, err := Marshal(session, secret)
		assert.NoErr(t, err)

		anotherSecret := hmactoken.NewStaticSecret(secrets.Secret{
			Active: []byte("another-secret"),
		})

		_, err = Unmarshal(blob, anotherSecret)
		assert.That(t, err, should.ErrLike("bad session token MAC"))
	})
}

func TestCreateUpdate(t *testing.T) {
	t.Parallel()

	testTime := time.Date(2100, time.December, 1, 2, 3, 4, 0, time.UTC)

	pb := Create(SessionParameters{
		SessionID: "session-id",
		BotID:     "bot-id",
		BotGroup: &cfg.BotGroup{
			Dimensions: map[string][]string{
				"pool": {"some-pool"},
				"key":  {"v2", "v1"},
			},
			Auth:                  []*configpb.BotAuth{{RequireLuciMachineToken: true}},
			SystemServiceAccount:  "system-service-account",
			BotConfigScriptName:   "bot-config-script-name",
			BotConfigScriptBody:   "bot-config-body",
			BotConfigScriptSHA256: "bot-config-sha256",
			LogsCloudProject:      "logs-cloud-project",
		},
		RBEConfig: cfg.RBEConfig{
			Instance: "rbe-instance",
		},
		ServerConfig: &cfg.Config{
			VersionInfo: cfg.VersionInfo{
				Fetched: testTime.Add(-time.Hour),
			},
		},
		DebugInfo: &internalspb.DebugInfo{
			SwarmingVersion: "server-ver",
			RequestId:       "request-id",
		},
		Now: testTime,
	})

	expectedHandshakeConfigHash := digest([]string{
		"config_script_name:bot-config-script-name",
		"config_script_sha256:bot-config-sha256",
		"dimension:key:v1",
		"dimension:key:v2",
		"dimension:pool:some-pool",
	})

	assert.That(t, pb, should.Match(&internalspb.Session{
		SessionId: "session-id",
		BotId:     "bot-id",
		Expiry:    timestamppb.New(testTime.Add(Expiry)),
		DebugInfo: &internalspb.DebugInfo{
			SwarmingVersion: "server-ver",
			RequestId:       "request-id",
		},
		BotConfig: &internalspb.BotConfig{
			Expiry: timestamppb.New(testTime.Add(Expiry)),
			DebugInfo: &internalspb.DebugInfo{
				SwarmingVersion: "server-ver",
				RequestId:       "request-id",
			},
			BotAuth:              []*configpb.BotAuth{{RequireLuciMachineToken: true}},
			SystemServiceAccount: "system-service-account",
			LogsCloudProject:     "logs-cloud-project",
			RbeInstance:          "rbe-instance",
		},
		HandshakeConfigHash: expectedHandshakeConfigHash,
		LastSeenConfig:      timestamppb.New(testTime.Add(-time.Hour)),
	}))

	testTime = testTime.Add(10 * time.Minute)

	pb = Update(pb, SessionParameters{
		BotGroup: &cfg.BotGroup{
			Dimensions: map[string][]string{
				"pool": {"another-pool"},
			},
			SystemServiceAccount: "another-system-service-account",
		},
		RBEConfig: cfg.RBEConfig{
			Instance:                "rbe-instance",
			EffectiveBotIDDimension: "effective-bot-id-dim",
		},
		RBEEffectiveBotID: "effective-bot-id-val",
		ServerConfig: &cfg.Config{
			VersionInfo: cfg.VersionInfo{
				Fetched: testTime.Add(-time.Hour),
			},
		},
		DebugInfo: &internalspb.DebugInfo{
			SwarmingVersion: "server-ver",
			RequestId:       "another-request-id",
		},
		Now: testTime,
	})

	assert.That(t, pb, should.Match(&internalspb.Session{
		SessionId: "session-id", // unchanged
		BotId:     "bot-id",     // unchanged
		Expiry:    timestamppb.New(testTime.Add(Expiry)),
		DebugInfo: &internalspb.DebugInfo{
			SwarmingVersion: "server-ver",
			RequestId:       "another-request-id",
		},
		BotConfig: &internalspb.BotConfig{
			Expiry: timestamppb.New(testTime.Add(Expiry)),
			DebugInfo: &internalspb.DebugInfo{
				SwarmingVersion: "server-ver",
				RequestId:       "another-request-id",
			},
			SystemServiceAccount:       "another-system-service-account",
			RbeInstance:                "rbe-instance",
			RbeEffectiveBotIdDimension: "effective-bot-id-dim",
			RbeEffectiveBotId:          "effective-bot-id-val",
		},
		HandshakeConfigHash: expectedHandshakeConfigHash, // unchanged
		LastSeenConfig:      timestamppb.New(testTime.Add(-time.Hour)),
	}))
}

func TestIsHandshakeConfigStale(t *testing.T) {
	t.Parallel()

	testTime := time.Date(2100, time.December, 1, 2, 3, 4, 0, time.UTC)

	botGroup := func() *cfg.BotGroup {
		return &cfg.BotGroup{
			Dimensions: map[string][]string{
				"pool": {"some-pool"},
				"key":  {"v2", "v1"},
			},
			BotConfigScriptName:   "bot-config-script-name",
			BotConfigScriptSHA256: "bot-config-sha256",
			SystemServiceAccount:  "system-service-account",
		}
	}

	pb := Create(SessionParameters{
		SessionID: "session-id",
		BotID:     "bot-id",
		BotGroup:  botGroup(),
		ServerConfig: &cfg.Config{
			VersionInfo: cfg.VersionInfo{
				Fetched: testTime.Add(-time.Hour),
			},
		},
		Now: testTime,
	})

	// Changing non-essential parameter doesn't invalidate the session.
	bg := botGroup()
	bg.SystemServiceAccount = "another"
	assert.That(t, IsHandshakeConfigStale(pb, bg), should.BeFalse)

	// Changing essential parameters does.
	bg = botGroup()
	bg.BotConfigScriptSHA256 = "another"
	assert.That(t, IsHandshakeConfigStale(pb, bg), should.BeTrue)
	bg = botGroup()
	bg.BotConfigScriptName = "another"
	assert.That(t, IsHandshakeConfigStale(pb, bg), should.BeTrue)
	bg = botGroup()
	bg.Dimensions["more"] = []string{"val"}
	assert.That(t, IsHandshakeConfigStale(pb, bg), should.BeTrue)
}

func digest(lines []string) []byte {
	blob := strings.Join(lines, "\n")
	h := sha256.New()
	_, _ = h.Write([]byte(blob))
	return h.Sum(nil)
}
