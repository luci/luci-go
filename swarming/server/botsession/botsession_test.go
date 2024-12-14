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
		assert.That(t, err, should.ErrLike(nil))

		restored, err := Unmarshal(blob, secret)
		assert.That(t, err, should.ErrLike(nil))

		assert.That(t, session, should.Match(restored))
	})

	t.Run("Bad MAC", func(t *testing.T) {
		blob, err := Marshal(session, secret)
		assert.That(t, err, should.ErrLike(nil))

		anotherSecret := hmactoken.NewStaticSecret(secrets.Secret{
			Active: []byte("another-secret"),
		})

		_, err = Unmarshal(blob, anotherSecret)
		assert.That(t, err, should.ErrLike("bad session token MAC"))
	})
}

func TestCreate(t *testing.T) {
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
		HandshakeConfigHash: digest([]string{
			"config_script_name:bot-config-script-name",
			"config_script_sha256:bot-config-sha256",
			"dimension:key:v1",
			"dimension:key:v2",
			"dimension:pool:some-pool",
		}),
		LastSeenConfig: timestamppb.New(testTime.Add(-time.Hour)),
	}))
}

func digest(lines []string) []byte {
	blob := strings.Join(lines, "\n")
	h := sha256.New()
	_, _ = h.Write([]byte(blob))
	return h.Sum(nil)
}
