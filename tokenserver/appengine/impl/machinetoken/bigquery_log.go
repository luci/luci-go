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
	"context"
	"math/big"
	"net"

	"google.golang.org/protobuf/types/known/timestamppb"

	tokenserver "go.chromium.org/luci/tokenserver/api"
	bqpb "go.chromium.org/luci/tokenserver/api/bq"
	"go.chromium.org/luci/tokenserver/api/minter/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/certconfig"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/bq"
)

func init() {
	bq.RegisterTokenKind("machine_tokens", (*bqpb.MachineToken)(nil))
}

// MintedTokenInfo is passed to LogToken.
//
// It carries all information about the token minting operation and the produced
// token.
type MintedTokenInfo struct {
	Request   *minter.MachineTokenRequest   // the token request, as presented by the client
	Response  *minter.MachineTokenResponse  // the response, as returned by the minter
	TokenBody *tokenserver.MachineTokenBody // deserialized token (same as in Response)
	CA        *certconfig.CA                // CA configuration used to authorize this request
	PeerIP    net.IP                        // caller IP address
	RequestID string                        // GAE request ID that handled the RPC
}

// toBigQueryMessage returns a message to upload to BigQuery.
func (i *MintedTokenInfo) toBigQueryMessage() *bqpb.MachineToken {
	// LUCI_MACHINE_TOKEN is the only supported type currently.
	if i.Request.TokenType != tokenserver.MachineTokenType_LUCI_MACHINE_TOKEN {
		panic("unknown token type")
	}
	return &bqpb.MachineToken{
		// Identifier of the token body.
		Fingerprint: utils.TokenFingerprint(i.Response.GetLuciMachineToken().MachineToken),

		// Information about the token.
		MachineFqdn:        i.TokenBody.MachineFqdn,
		TokenType:          i.Request.TokenType,
		IssuedAt:           &timestamppb.Timestamp{Seconds: int64(i.TokenBody.IssuedAt)},
		Expiration:         &timestamppb.Timestamp{Seconds: int64(i.TokenBody.IssuedAt + i.TokenBody.Lifetime)},
		CertSerialNumber:   new(big.Int).SetBytes(i.TokenBody.CertSn).String(),
		SignatureAlgorithm: i.Request.SignatureAlgorithm,

		// Information about the CA used to authorize this request.
		CaCommonName: i.CA.CN,
		CaConfigRev:  i.CA.UpdatedRev,

		// Information about the request handler.
		PeerIp:         i.PeerIP.String(),
		ServiceVersion: i.Response.ServiceVersion,
		GaeRequestId:   i.RequestID,
	}
}

// TokenLogger records info about the token to BigQuery.
type TokenLogger func(context.Context, *MintedTokenInfo) error

// NewTokenLogger returns a callback that records info about tokens to BigQuery.
//
// Tokens themselves are not logged. Only first 16 bytes of their SHA256 hashes
// (aka 'fingerprint') are. They are used only to identify tokens in logs.
//
// When dryRun is true, logs to the local text log only, not to BigQuery
// (to avoid accidentally pushing fake data to real BigQuery dataset).
func NewTokenLogger(dryRun bool) TokenLogger {
	return func(ctx context.Context, i *MintedTokenInfo) error {
		return bq.LogToken(ctx, i.toBigQueryMessage(), dryRun)
	}
}
