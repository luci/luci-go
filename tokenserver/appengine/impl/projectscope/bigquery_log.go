// Copyright 2019 The LUCI Authors.
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

package projectscope

import (
	"context"
	"net"

	"google.golang.org/protobuf/types/known/timestamppb"

	"cloud.google.com/go/bigquery"

	"go.chromium.org/luci/auth/identity"

	bqpb "go.chromium.org/luci/tokenserver/api/bq"
	"go.chromium.org/luci/tokenserver/api/minter/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/bq"
)

// MintedTokenInfo is passed to LogToken.
//
// It carries all information about the token minting operation and the produced
// token.
type MintedTokenInfo struct {
	Request      *minter.MintProjectTokenRequest  // RPC input, as is
	Response     *minter.MintProjectTokenResponse // RPC output, as is
	RequestedAt  *timestamppb.Timestamp
	Expiration   *timestamppb.Timestamp
	PeerIdentity identity.Identity // caller identity
	PeerIP       net.IP            // caller IP address
	RequestID    string            // GAE request ID that handled the RPC
	AuthDBRev    int64             // revision of groups database (or 0 if unknown)
}

// toBigQueryMessage returns a message to upload to BigQuery.
func (i *MintedTokenInfo) toBigQueryMessage() *bqpb.ProjectToken {
	return &bqpb.ProjectToken{
		// Information about the produced token.
		Fingerprint:     utils.TokenFingerprint(i.Response.AccessToken),
		ServiceAccount:  i.Response.ServiceAccountEmail,
		OauthScopes:     i.Request.OauthScope,
		LuciProject:     i.Request.LuciProject,
		ServiceIdentity: i.PeerIdentity.Email(),
		RequestedAt:     i.RequestedAt,
		Expiration:      i.Expiration,
		AuditTags:       i.Request.AuditTags,
		PeerIp:          i.PeerIP.String(),
		ServiceVersion:  i.Response.ServiceVersion,
		GaeRequestId:    i.RequestID,
		AuthDbRev:       i.AuthDBRev,
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
func NewTokenLogger(client *bigquery.Client, dryRun bool) TokenLogger {
	inserter := bq.Inserter{
		Table:  client.Dataset("tokens").Table("project_tokens"),
		DryRun: dryRun,
	}
	return func(ctx context.Context, i *MintedTokenInfo) error {
		return inserter.Insert(ctx, i.toBigQueryMessage())
	}
}
