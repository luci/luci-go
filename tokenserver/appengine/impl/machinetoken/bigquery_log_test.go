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
	"math/big"
	"net"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	tokenserver "go.chromium.org/luci/tokenserver/api"
	bqpb "go.chromium.org/luci/tokenserver/api/bq"
	"go.chromium.org/luci/tokenserver/api/minter/v1"
)

func TestMintedTokenInfo(t *testing.T) {
	ftt.Run("produces correct row map", t, func(t *ftt.Test) {
		ctx := testingContext(testingCA)

		info := MintedTokenInfo{
			Request: testingRawRequest(ctx),
			Response: &minter.MachineTokenResponse{
				ServiceVersion: "unit-tests/mocked-ver",
				TokenType: &minter.MachineTokenResponse_LuciMachineToken{
					LuciMachineToken: &minter.LuciMachineToken{
						MachineToken: "zzzz",
						Expiry:       timestamppb.New(clock.Now(ctx).Add(time.Hour)),
					},
				},
			},
			TokenBody: &tokenserver.MachineTokenBody{
				MachineFqdn: "luci-token-server-test-1.fake.domain",
				IssuedBy:    "signer@testing.host",
				IssuedAt:    1422936306,
				Lifetime:    3600,
				CaId:        123,
				CertSn:      big.NewInt(4096).Bytes(),
			},
			CA:        &testingCA,
			PeerIP:    net.ParseIP("127.10.10.10"),
			RequestID: "gae-request-id",
		}

		assert.Loosely(t, info.toBigQueryMessage(), should.Match(&bqpb.MachineToken{
			CaCommonName:       "Fake CA: fake.ca",
			CaConfigRev:        "cfg-updated-rev",
			CertSerialNumber:   "4096",
			Expiration:         &timestamppb.Timestamp{Seconds: 1422939906},
			Fingerprint:        "2d6ccd34ad7af363159ed4bbe18c0e43",
			GaeRequestId:       "gae-request-id",
			IssuedAt:           &timestamppb.Timestamp{Seconds: 1422936306},
			MachineFqdn:        "luci-token-server-test-1.fake.domain",
			PeerIp:             "127.10.10.10",
			ServiceVersion:     "unit-tests/mocked-ver",
			SignatureAlgorithm: minter.SignatureAlgorithm_SHA256_RSA_ALGO,
			TokenType:          tokenserver.MachineTokenType_LUCI_MACHINE_TOKEN,
		}))
	})
}
