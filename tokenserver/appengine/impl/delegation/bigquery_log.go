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
	"context"
	"fmt"
	"net"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
	bqpb "go.chromium.org/luci/tokenserver/api/bq"
	"go.chromium.org/luci/tokenserver/api/minter/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/bq"
)

func init() {
	bq.RegisterTokenKind("delegation_tokens", (*bqpb.DelegationToken)(nil))
}

// MintedTokenInfo is passed to LogToken.
//
// It carries all information about the token minting operation and the produced
// token.
type MintedTokenInfo struct {
	Request   *minter.MintDelegationTokenRequest  // RPC input, as is
	Response  *minter.MintDelegationTokenResponse // RPC output, as is
	ConfigRev string                              // revision of the delegation.cfg used
	Rule      *admin.DelegationRule               // the particular rule used to authorize the request
	PeerIP    net.IP                              // caller IP address
	RequestID string                              // GAE request ID that handled the RPC
	AuthDBRev int64                               // revision of groups database (or 0 if unknown)
}

// toBigQueryMessage returns a message to upload to BigQuery.
func (i *MintedTokenInfo) toBigQueryMessage() *bqpb.DelegationToken {
	subtok := i.Response.DelegationSubtoken
	return &bqpb.DelegationToken{
		// Information about the produced token.
		Fingerprint:       utils.TokenFingerprint(i.Response.Token),
		TokenKind:         subtok.Kind,
		TokenId:           fmt.Sprintf("%d", subtok.SubtokenId),
		DelegatedIdentity: subtok.DelegatedIdentity,
		RequestorIdentity: subtok.RequestorIdentity,
		IssuedAt:          &timestamppb.Timestamp{Seconds: subtok.CreationTime},
		Expiration:        &timestamppb.Timestamp{Seconds: subtok.CreationTime + int64(subtok.ValidityDuration)},
		TargetAudience:    subtok.Audience,
		TargetServices:    subtok.Services,

		// Information about the request.
		RequestedValidity: i.Request.ValidityDuration,
		RequestedIntent:   i.Request.Intent,
		Tags:              subtok.Tags,

		// Information about the delegation rule used.
		ConfigRev:  i.ConfigRev,
		ConfigRule: i.Rule.Name,

		// Information about the request handler environment.
		PeerIp:         i.PeerIP.String(),
		ServiceVersion: i.Response.ServiceVersion,
		GaeRequestId:   i.RequestID,
		AuthDbRev:      i.AuthDBRev,
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
