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

func TestMintedGrantInfo(t *testing.T) {
	t.Parallel()

	Convey("Produces correct row map", t, func() {
		info := MintedGrantInfo{
			Request: &minter.MintOAuthTokenGrantRequest{
				ServiceAccount:   "service-account@robots.com",
				ValidityDuration: 3600,
				EndUser:          "user:end-user@example.com",
				AuditTags:        []string{"k1:v1", "k2:v2"},
			},
			Response: &minter.MintOAuthTokenGrantResponse{
				GrantToken:     "blah",
				ServiceVersion: "unit-tests/mocked-ver",
			},
			GrantBody: &tokenserver.OAuthTokenGrantBody{
				TokenId:          1234,
				ServiceAccount:   "service-account@robots.com",
				Proxy:            "user:proxy@example.com",
				EndUser:          "user:end-user@example.com",
				IssuedAt:         google.NewTimestamp(time.Date(2015, time.February, 3, 4, 5, 6, 0, time.UTC)),
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

		So(info.toBigQueryMessage(), ShouldResemble, &bqpb.OAuthTokenGrant{
			AuditTags:       []string{"k1:v1", "k2:v2"},
			AuthDbRev:       123,
			ConfigRev:       "config-rev",
			ConfigRule:      "rule-name",
			EndUserIdentity: "user:end-user@example.com",
			Expiration:      &timestamp.Timestamp{Seconds: 1422939906},
			Fingerprint:     "8b7df143d91c716ecfa5fc1730022f6b",
			GaeRequestId:    "gae-request-id",
			IssuedAt:        &timestamp.Timestamp{Seconds: 1422936306},
			PeerIp:          "127.10.10.10",
			ProxyIdentity:   "user:proxy@example.com",
			ServiceAccount:  "service-account@robots.com",
			ServiceVersion:  "unit-tests/mocked-ver",
			TokenId:         "1234",
		})
	})
}
