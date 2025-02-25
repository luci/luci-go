// Copyright 2020 The LUCI Authors.
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

package serviceaccounts

import (
	"net"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	bqpb "go.chromium.org/luci/tokenserver/api/bq"
	"go.chromium.org/luci/tokenserver/api/minter/v1"
)

func TestMintedTokenInfo(t *testing.T) {
	t.Parallel()

	ftt.Run("Conversion to row", t, func(t *ftt.Test) {
		info := MintedTokenInfo{
			Request: &minter.MintServiceAccountTokenRequest{
				TokenKind:       minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
				ServiceAccount:  "acc@example.com",
				Realm:           "proj:realm",
				OauthScope:      []string{"ignored"},
				IdTokenAudience: "aud",
				AuditTags:       []string{"k:v"},
			},
			Response: &minter.MintServiceAccountTokenResponse{
				Token:          "some-token",
				Expiry:         &timestamppb.Timestamp{Seconds: 123456},
				ServiceVersion: "unit-tests/mocked-ver",
			},
			RequestedAt:     time.Unix(1234, 0),
			OAuthScopes:     []string{"a", "b"},
			RequestIdentity: "user:req@example.com",
			PeerIdentity:    "user:peer@example.com",
			ConfigRev:       "config-rev",
			PeerIP:          net.ParseIP("127.1.1.1"),
			RequestID:       "request-id",
			AuthDBRev:       111,
		}

		assert.Loosely(t, info.toBigQueryMessage(), should.Match(&bqpb.ServiceAccountToken{
			Fingerprint:     "308eda9daf26b7446b284449a5895ab9",
			Kind:            minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
			ServiceAccount:  "acc@example.com",
			Realm:           "proj:realm",
			OauthScopes:     []string{"a", "b"},
			IdTokenAudience: "aud",
			RequestIdentity: "user:req@example.com",
			PeerIdentity:    "user:peer@example.com",
			RequestedAt:     &timestamppb.Timestamp{Seconds: 1234},
			Expiration:      &timestamppb.Timestamp{Seconds: 123456},
			AuditTags:       []string{"k:v"},
			ConfigRev:       "config-rev",
			PeerIp:          "127.1.1.1",
			ServiceVersion:  "unit-tests/mocked-ver",
			GaeRequestId:    "request-id",
			AuthDbRev:       111,
		}))
	})
}
