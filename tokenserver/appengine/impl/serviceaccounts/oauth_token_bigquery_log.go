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
	"context"
	"net"
	"time"

	"google.golang.org/appengine"

	"go.chromium.org/luci/appengine/bqlog"
	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/proto/google"

	"go.chromium.org/luci/tokenserver/api"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
	bqpb "go.chromium.org/luci/tokenserver/api/bq"
	"go.chromium.org/luci/tokenserver/api/minter/v1"

	"go.chromium.org/luci/tokenserver/appengine/impl/utils"
)

var oauthTokensLog = bqlog.Log{
	QueueName:           "bqlog-oauth-tokens", // see queues.yaml
	DatasetID:           "tokens",             // see bq/README.md
	TableID:             "oauth_tokens",       // see bq/tables/oauth_tokens.schema
	DumpEntriesToLogger: true,
	DryRun:              appengine.IsDevAppServer(),
}

// MintedOAuthTokenInfo is passed to LogOAuthToken.
//
// It carries all information about the returned token.
type MintedOAuthTokenInfo struct {
	RequestedAt time.Time                              // when the RPC happened
	Request     *minter.MintOAuthTokenViaGrantRequest  // RPC input, as is
	Response    *minter.MintOAuthTokenViaGrantResponse // RPC output, as is
	GrantBody   *tokenserver.OAuthTokenGrantBody       // deserialized grant
	ConfigRev   string                                 // revision of the service_accounts.cfg used
	Rule        *admin.ServiceAccountRule              // the particular rule used to authorize the request
	PeerIP      net.IP                                 // caller IP address
	RequestID   string                                 // GAE request ID that handled the RPC
	AuthDBRev   int64                                  // revision of groups database (or 0 if unknown)
}

// toBigQueryMessage returns a message to upload to BigQuery.
func (i *MintedOAuthTokenInfo) toBigQueryMessage() *bqpb.OAuthToken {
	return &bqpb.OAuthToken{
		Fingerprint:      utils.TokenFingerprint(i.Response.AccessToken),
		GrantFingerprint: utils.TokenFingerprint(i.Request.GrantToken),
		ServiceAccount:   i.GrantBody.ServiceAccount,
		OauthScopes:      i.Request.OauthScope,
		ProxyIdentity:    i.GrantBody.Proxy,
		EndUserIdentity:  i.GrantBody.EndUser,

		// Note: we are not using 'issued_at' because the returned token is often
		// fetched from cache (and thus it was issued some time ago, not now). This
		// timestamp is not preserved in the cache, since it can be calculated from
		// 'expiration' if necessary.
		RequestedAt: google.NewTimestamp(i.RequestedAt),
		Expiration:  i.Response.Expiry,

		// Information supplied by the caller.
		AuditTags: i.Request.AuditTags,

		// Information about the service account rule used.
		ConfigRev:  i.ConfigRev,
		ConfigRule: i.Rule.Name,

		// Information about the request handler environment.
		PeerIp:         i.PeerIP.String(),
		ServiceVersion: i.Response.ServiceVersion,
		GaeRequestId:   i.RequestID,
		AuthDbRev:      i.AuthDBRev,
	}
}

// LogOAuthToken records information about the OAuth token in the BigQuery.
//
// The token itself is not logged. Only first 16 bytes of its SHA256 hash
// (aka 'fingerprint') is. It is used only to identify this particular token in
// logs.
//
// On dev server, logs to the GAE log only, not to BigQuery (to avoid
// accidentally pushing fake data to real BigQuery dataset).
func LogOAuthToken(c context.Context, i *MintedOAuthTokenInfo) error {
	return oauthTokensLog.Insert(c, &bq.Row{
		Message: i.toBigQueryMessage(),
	})
}

// FlushOAuthTokensLog sends all buffered logged tokens to BigQuery.
//
// It is fine to call FlushOAuthTokensLog concurrently from multiple request
// handlers, if necessary (it will effectively parallelize the flush).
func FlushOAuthTokensLog(c context.Context) error {
	_, err := oauthTokensLog.Flush(c)
	return err
}
