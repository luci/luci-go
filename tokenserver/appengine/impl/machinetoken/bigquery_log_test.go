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

package machinetoken

import (
	"net"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/tokenserver/api"
	"go.chromium.org/luci/tokenserver/api/minter/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMintedTokenInfo(t *testing.T) {
	Convey("produces correct row map", t, func() {
		ctx := testingContext(testingCA)

		info := MintedTokenInfo{
			Request: testingRawRequest(ctx),
			Response: &minter.MachineTokenResponse{
				ServiceVersion: "unit-tests/mocked-ver",
				TokenType: &minter.MachineTokenResponse_LuciMachineToken{
					LuciMachineToken: &minter.LuciMachineToken{
						MachineToken: "zzzz",
						Expiry:       google.NewTimestamp(clock.Now(ctx).Add(time.Hour)),
					},
				},
			},
			TokenBody: &tokenserver.MachineTokenBody{
				MachineFqdn: "luci-token-server-test-1.fake.domain",
				IssuedBy:    "signer@testing.host",
				IssuedAt:    1422936306,
				Lifetime:    3600,
				CaId:        123,
				CertSn:      4096,
			},
			CA:        &testingCA,
			PeerIP:    net.ParseIP("127.10.10.10"),
			RequestID: "gae-request-id",
		}

		So(info.toBigQueryRow(), ShouldResemble, map[string]interface{}{
			"ca_common_name":      "Fake CA: fake.ca",
			"ca_config_rev":       "cfg-updated-rev",
			"cert_serial_number":  "4096",
			"expiration":          1.422939906e+09,
			"fingerprint":         "2d6ccd34ad7af363159ed4bbe18c0e43",
			"gae_request_id":      "gae-request-id",
			"issued_at":           1.422936306e+09,
			"machine_fqdn":        "luci-token-server-test-1.fake.domain",
			"peer_ip":             "127.10.10.10",
			"service_version":     "unit-tests/mocked-ver",
			"signature_algorithm": "SHA256_RSA_ALGO",
			"token_type":          "LUCI_MACHINE_TOKEN",
		})
	})
}
