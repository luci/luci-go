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

package serviceaccountsv2

import (
	"context"
	"net"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/proto/google"

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
	Request         *minter.MintServiceAccountTokenRequest  // RPC input, as is
	Response        *minter.MintServiceAccountTokenResponse // RPC output, as is
	RequestedAt     time.Time
	OAuthScopes     []string          // normalized list of requested OAuth scopes
	RequestIdentity identity.Identity // identity used in authorization
	PeerIdentity    identity.Identity // identity of the direct peer
	ConfigRev       string            // revision of the service config
	PeerIP          net.IP            // caller's IP
	RequestID       string            // GAE request ID that handles the RPC
	AuthDBRev       int64             // revision of the authorization database
}

// toBigQueryMessage returns a message to upload to BigQuery.
func (i *MintedTokenInfo) toBigQueryMessage() *bqpb.ServiceAccountToken {
	return &bqpb.ServiceAccountToken{
		Fingerprint:     utils.TokenFingerprint(i.Response.Token),
		Kind:            i.Request.TokenKind,
		ServiceAccount:  i.Request.ServiceAccount,
		Realm:           i.Request.Realm,
		OauthScopes:     i.OAuthScopes,
		IdTokenAudience: i.Request.IdTokenAudience,
		RequestIdentity: string(i.RequestIdentity),
		PeerIdentity:    string(i.PeerIdentity),
		RequestedAt:     google.NewTimestamp(i.RequestedAt),
		Expiration:      i.Response.Expiry,
		AuditTags:       i.Request.AuditTags,
		ConfigRev:       i.ConfigRev,
		PeerIp:          i.PeerIP.String(),
		ServiceVersion:  i.Response.ServiceVersion,
		GaeRequestId:    i.RequestID,
		AuthDbRev:       i.AuthDBRev,
	}
}

// LogToken records information about the token in the BigQuery.
//
// The token itself is not logged. Only first 16 bytes of its SHA256 hash
// (aka 'fingerprint') is. It is used only to identify this particular token in
// logs.
//
// On dev server, logs to the GAE log only, not to BigQuery (to avoid
// accidentally pushing fake data to real BigQuery dataset).
func LogToken(ctx context.Context, i *MintedTokenInfo) error {
	return bq.InsertFromGAEv1(ctx, "tokens", "service_account_tokens", i.toBigQueryMessage())
}
