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

package serviceaccounts

import (
	"net"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes/timestamp"

	"go.chromium.org/luci/common/proto/google"

	"go.chromium.org/luci/tokenserver/api"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
	bqpb "go.chromium.org/luci/tokenserver/api/bq"
	"go.chromium.org/luci/tokenserver/api/minter/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMintedOAuthTokenInfo(t *testing.T) {
	t.Parallel()

	Convey("Produces correct row map", t, func() {
		epoch := time.Date(2015, time.February, 1, 2, 3, 4, 5, time.UTC)

		info := MintedOAuthTokenInfo{
			RequestedAt: epoch.Add(30 * time.Minute),
			Request: &minter.MintOAuthTokenViaGrantRequest{
				GrantToken: "grant-token",
				OauthScope: []string{"https://scope1", "https://scope2"},
				AuditTags:  []string{"k1:v1", "k2:v2"},
			},
			Response: &minter.MintOAuthTokenViaGrantResponse{
				AccessToken:    "access-token",
				Expiry:         google.NewTimestamp(epoch.Add(time.Hour)),
				ServiceVersion: "unit-tests/mocked-ver",
			},
			GrantBody: &tokenserver.OAuthTokenGrantBody{
				TokenId:          1234,
				ServiceAccount:   "service-account@robots.com",
				Proxy:            "user:proxy@example.com",
				EndUser:          "user:end-user@example.com",
				IssuedAt:         google.NewTimestamp(epoch),
				ValidityDuration: 3600,
			},
			ConfigRev: "config-rev",
			Rule: &admin.ServiceAccountRule{
				Name: "rule-name",
			},
			PeerIP:    net.ParseIP("127.10.10.10"),
			RequestID: "gae-request-id",
			AuthDBRev: 123,
		}

		So(info.toBigQueryMessage(), ShouldResemble, &bqpb.OAuthToken{
			AuditTags:        []string{"k1:v1", "k2:v2"},
			AuthDbRev:        123,
			ConfigRev:        "config-rev",
			ConfigRule:       "rule-name",
			EndUserIdentity:  "user:end-user@example.com",
			Expiration:       &timestamp.Timestamp{Seconds: 1422759784, Nanos: 5},
			Fingerprint:      "3f16bed7089f4653e5ef21bfd2824d7f",
			GaeRequestId:     "gae-request-id",
			GrantFingerprint: "6d2bfc0147054b3d0ad9dac8d06b6f65",
			OauthScopes:      []string{"https://scope1", "https://scope2"},
			PeerIp:           "127.10.10.10",
			ProxyIdentity:    "user:proxy@example.com",
			RequestedAt:      &timestamp.Timestamp{Seconds: 1422757984, Nanos: 5},
			ServiceAccount:   "service-account@robots.com",
			ServiceVersion:   "unit-tests/mocked-ver",
		})
	})
}
