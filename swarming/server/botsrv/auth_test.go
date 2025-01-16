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
	"context"
	"fmt"
	"net"
	"testing"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/tokenserver/auth/machine"

	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/metrics"
)

func TestAuthorizeBot(t *testing.T) {
	t.Parallel()

	var (
		goodTestIP = net.ParseIP("192.0.2.1")
		badTestIP  = net.ParseIP("192.0.2.2")
	)
	const (
		ipAllowlist = "ip-allowlist"
	)

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx := memlogger.Use(context.Background())
		ctx, _ = tsmon.WithDummyInMemory(ctx)

		authDB := authtest.NewFakeDB(
			authtest.MockIPAllowlist(goodTestIP.String(), ipAllowlist),
		)

		logs := func() []string {
			entries := logging.Get(ctx).(*memlogger.MemLogger).Messages()
			logs := make([]string, len(entries))
			for i, e := range entries {
				logs[i] = e.Msg
			}
			return logs
		}

		popMetric := func() (out string) {
			store := tsmon.Store(ctx)
			cells := store.GetAll(ctx)
			for _, cell := range cells {
				if cell.MetricInfo.Name == "swarming/bot_auth/success" {
					out += fmt.Sprintf("%s:%s", cell.FieldVals[0], cell.FieldVals[1])
				}
			}
			store.Reset(ctx, metrics.BotAuthSuccesses)
			return
		}

		t.Run("No creds, all methods", func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				FakeDB:         authDB,
				Identity:       identity.AnonymousIdentity,
				PeerIPOverride: badTestIP,
			})
			err := AuthorizeBot(ctx, "bot-id", []*configpb.BotAuth{
				{RequireLuciMachineToken: true},
				{RequireGceVmToken: &configpb.BotAuth_GCE{Project: "gce-proj"}},
				{RequireServiceAccount: []string{"some@example.com"}},
				{IpWhitelist: ipAllowlist},
			})
			assert.Loosely(t, err,
				should.ErrLike(
					"all auth methods failed: no LUCI machine token in the request; "+
						"no GCE VM token in the request; "+
						"no OAuth or OpenID credentials in the request; "+
						"IP not allowed",
				),
			)
			assert.Loosely(t, logs(), should.Match([]string{
				"Bot ID: bot-id",
				`Auth method {"requireLuciMachineToken":true}: no LUCI machine token in the request`,
				`Auth method {"requireGceVmToken":{"project":"gce-proj"}}: no GCE VM token in the request`,
				`Auth method {"requireServiceAccount":["some@example.com"]}: no OAuth or OpenID credentials in the request`,
				`Auth method {"ipWhitelist":"ip-allowlist"}: IP not allowed`,
				`Bot IP 192.0.2.2 is not in the allowlist "ip-allowlist"`,
			}))
			assert.That(t, popMetric(), should.Equal(""))
		})

		t.Run("Silently skips fails on success", func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				FakeDB:   authDB,
				Identity: "user:good@example.com",
			})
			err := AuthorizeBot(ctx, "bot-id", []*configpb.BotAuth{
				{RequireServiceAccount: []string{"bad@example.com"}},
				{RequireServiceAccount: []string{"good@example.com"}},
			})
			assert.NoErr(t, err)
			assert.Loosely(t, logs(), should.Match([]string{}))
		})

		t.Run("Logs fails if LogIfFailed==true", func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				FakeDB:   authDB,
				Identity: "user:good@example.com",
			})
			err := AuthorizeBot(ctx, "bot-id", []*configpb.BotAuth{
				{RequireServiceAccount: []string{"bad@example.com"}, LogIfFailed: true},
				{RequireServiceAccount: []string{"good@example.com"}},
			})
			assert.NoErr(t, err)
			assert.Loosely(t, logs(), should.Match([]string{
				"Bot ID: bot-id",
				`Preferred auth method {"logIfFailed":true,"requireServiceAccount":["bad@example.com"]}: the host "bot-id" is authenticated as "good@example.com" which is not the expected service account for this host`,
				`Another auth method succeeded {"requireServiceAccount":["good@example.com"]}`,
			}))
		})

		t.Run("IP allowlist as an extra check", func(t *ftt.Test) {
			t.Run("Pass", func(t *ftt.Test) {
				ctx := auth.WithState(ctx, &authtest.FakeState{
					FakeDB:         authDB,
					Identity:       "user:some@example.com",
					PeerIPOverride: goodTestIP,
				})
				err := AuthorizeBot(ctx, "bot-id", []*configpb.BotAuth{
					{
						RequireServiceAccount: []string{"some@example.com"},
						IpWhitelist:           ipAllowlist,
					},
				})
				assert.NoErr(t, err)
				assert.Loosely(t, logs(), should.Match([]string{}))
				assert.That(t, popMetric(), should.Equal("service_account:some@example.com"))
			})

			t.Run("Fail", func(t *ftt.Test) {
				ctx := auth.WithState(ctx, &authtest.FakeState{
					FakeDB:         authDB,
					Identity:       "user:some@example.com",
					PeerIPOverride: badTestIP,
				})
				err := AuthorizeBot(ctx, "bot-id", []*configpb.BotAuth{
					{
						RequireServiceAccount: []string{"some@example.com"},
						IpWhitelist:           ipAllowlist,
					},
				})
				assert.Loosely(t, err, should.ErrLike("IP not allowed"))
				assert.Loosely(t, logs(), should.Match([]string{
					"Bot ID: bot-id",
					`Auth method {"requireServiceAccount":["some@example.com"],"ipWhitelist":"ip-allowlist"}: IP not allowed`,
					`Bot IP 192.0.2.2 is not in the allowlist "ip-allowlist"`,
				}))
				assert.That(t, popMetric(), should.Equal(""))
			})
		})

		t.Run("LUCI machine token auth", func(t *ftt.Test) {
			cases := []struct {
				botID, fqdn string
				metric      string
				err         any
			}{
				{"bot-id", "bot-id.domain", "luci_token:-", nil},
				{"bot-id", "bot-id.domain.deeper", "luci_token:-", nil},
				{"bot-id", "bot-id", "luci_token:-", nil},
				{"bot-id--device123", "bot-id.domain", "luci_token:-", nil},
				{"bot-id--device123", "bot-id", "luci_token:-", nil},
				{"bot1", "bot2", "", `host ID "bot1" doesn't match the LUCI token with FQDN "bot2"`},
				{"bot1--device", "bot2", "", `host ID "bot1" doesn't match the LUCI token with FQDN "bot2"`},
				{"bot1", "bot12", "", `host ID "bot1" doesn't match the LUCI token with FQDN "bot12"`},
			}
			for _, cs := range cases {
				ctx := auth.WithState(ctx, &authtest.FakeState{
					FakeDB:    authDB,
					Identity:  "bot:unused",
					UserExtra: &machine.MachineTokenInfo{FQDN: cs.fqdn},
				})
				err := AuthorizeBot(ctx, cs.botID, []*configpb.BotAuth{
					{RequireLuciMachineToken: true},
				})
				assert.Loosely(t, err, should.ErrLike(cs.err))
				assert.That(t, popMetric(), should.Equal(cs.metric))
			}
		})

		t.Run("GCE VM token auth", func(t *ftt.Test) {
			cases := []struct {
				botID, expectedProj string
				instance, project   string
				metric              string
				err                 any
			}{
				{"id1", "proj1", "id1", "proj1", "gce_vm_token:proj1", nil},
				{"id1--device1", "proj1", "id1", "proj1", "gce_vm_token:proj1", nil},
				{"id1", "proj1", "id2", "proj1", "", `expecting VM token from GCE instance "id1", but got one from "id2"`},
				{"id1", "proj1", "id1", "proj2", "", `"proj2" is not an expected GCE project for host "id1"`},
				{"id1--device", "proj1", "id1", "proj2", "", `"proj2" is not an expected GCE project for host "id1"`},
			}
			for _, cs := range cases {
				ctx := auth.WithState(ctx, &authtest.FakeState{
					FakeDB:   authDB,
					Identity: "bot:unused",
					UserExtra: &openid.GoogleComputeTokenInfo{
						Instance: cs.instance,
						Project:  cs.project,
					},
				})
				err := AuthorizeBot(ctx, cs.botID, []*configpb.BotAuth{
					{RequireGceVmToken: &configpb.BotAuth_GCE{Project: cs.expectedProj}},
				})
				assert.Loosely(t, err, should.ErrLike(cs.err))
				assert.That(t, popMetric(), should.Equal(cs.metric))
			}
		})

		t.Run("Service account auth", func(t *ftt.Test) {
			cases := []struct {
				botID          string
				authenticateAs identity.Identity
				expectedEmails []string
				metric         string
				err            any
			}{
				{"bot1", "user:some@example.com", []string{"another@example.com", "some@example.com"}, "service_account:some@example.com", nil},
				{"bot1", "user:some@example.com", []string{"another@example.com"}, "",
					`the host "bot1" is authenticated as "some@example.com" which is not the expected service account for this host`,
				},
				{"bot1", "anonymous:anonymous", []string{"another@example.com"}, "",
					`no OAuth or OpenID credentials in the request`,
				},
			}
			for _, cs := range cases {
				ctx := auth.WithState(ctx, &authtest.FakeState{
					FakeDB:   authDB,
					Identity: cs.authenticateAs,
				})
				err := AuthorizeBot(ctx, cs.botID, []*configpb.BotAuth{
					{RequireServiceAccount: cs.expectedEmails},
				})
				assert.Loosely(t, err, should.ErrLike(cs.err))
				assert.That(t, popMetric(), should.Equal(cs.metric))
			}
		})

		t.Run("IP allowlist auth", func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				FakeDB:         authDB,
				Identity:       "user:some@example.com",
				PeerIPOverride: goodTestIP,
			})
			err := AuthorizeBot(ctx, "bot-id", []*configpb.BotAuth{
				{IpWhitelist: ipAllowlist},
			})
			assert.NoErr(t, err)
			assert.Loosely(t, logs(), should.Match([]string{}))
			assert.That(t, popMetric(), should.Equal("ip_allowlist:ip-allowlist"))
		})
	})
}
