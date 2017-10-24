// Copyright 2017 The LUCI Authors.
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

package delegation

import (
	"net"
	"testing"

	"go.chromium.org/luci/server/auth/delegation/messages"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/api/minter/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMintedTokenInfo(t *testing.T) {
	t.Parallel()

	Convey("produces correct row map", t, func() {
		info := MintedTokenInfo{
			Request: &minter.MintDelegationTokenRequest{
				ValidityDuration: 3600,
				Intent:           "intent string",
				Tags:             []string{"k:v"},
			},
			Response: &minter.MintDelegationTokenResponse{
				Token:          "blah",
				ServiceVersion: "unit-tests/mocked-ver",
				DelegationSubtoken: &messages.Subtoken{
					Kind:              messages.Subtoken_BEARER_DELEGATION_TOKEN,
					SubtokenId:        1234,
					DelegatedIdentity: "user:delegated@example.com",
					RequestorIdentity: "user:requestor@example.com",
					CreationTime:      1422936306,
					ValidityDuration:  3600,
					Audience:          []string{"user:audience@example.com"},
					Services:          []string{"*"},
					Tags:              []string{"k:v"},
				},
			},
			ConfigRev: "config-rev",
			Rule: &admin.DelegationRule{
				Name: "rule-name",
			},
			PeerIP:    net.ParseIP("127.10.10.10"),
			RequestID: "gae-request-id",
			AuthDBRev: 123,
		}

		So(info.toBigQueryRow(), ShouldResemble, map[string]interface{}{
			"auth_db_rev":        int64(123), // convey is pedantic about types
			"config_rev":         "config-rev",
			"config_rule":        "rule-name",
			"delegated_identity": "user:delegated@example.com",
			"expiration":         1.422939906e+09,
			"fingerprint":        "8b7df143d91c716ecfa5fc1730022f6b",
			"gae_request_id":     "gae-request-id",
			"issued_at":          1.422936306e+09,
			"peer_ip":            "127.10.10.10",
			"requested_intent":   "intent string",
			"requested_validity": int(3600),
			"requestor_identity": "user:requestor@example.com",
			"service_version":    "unit-tests/mocked-ver",
			"tags":               []string{"k:v"},
			"target_audience":    []string{"user:audience@example.com"},
			"target_services":    []string{"*"},
			"token_id":           "1234",
			"token_kind":         "BEARER_DELEGATION_TOKEN",
		})
	})
}
