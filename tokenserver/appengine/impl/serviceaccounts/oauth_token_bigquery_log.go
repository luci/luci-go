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
	"encoding/json"
	"net"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/info"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"

	"go.chromium.org/luci/tokenserver/api"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/api/minter/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/bqlog"
)

var oauthTokensLog = bqlog.Log{
	QueueName: "bqlog-oauth-tokens", // see queues.yaml
	DatasetID: "tokens",             // see bq/README.md
	TableID:   "oauth_tokens",       // see bq/tables/oauth_tokens.schema
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

// toBigQueryRow returns a JSON-ish map to upload to BigQuery.
//
// Its schema must match 'bq/tables/oauth_tokens.schema'.
func (i *MintedOAuthTokenInfo) toBigQueryRow() map[string]interface{} {
	return map[string]interface{}{
		"fingerprint":       utils.TokenFingerprint(i.Response.AccessToken),
		"grant_fingerprint": utils.TokenFingerprint(i.Request.GrantToken),
		"service_account":   i.GrantBody.ServiceAccount,
		"oauth_scopes":      i.Request.OauthScope,
		"proxy_identity":    i.GrantBody.Proxy,
		"end_user_identity": i.GrantBody.EndUser,

		// Note: we are not using 'issued_at' because the returned token is often
		// fetched from cache (and thus it was issued some time ago, not now). This
		// timestamp is not preserved in the cache, since it can be calculated from
		// 'expiration' if necessary.
		"requested_at": float64(i.RequestedAt.Unix()),
		"expiration":   float64(google.TimeFromProto(i.Response.Expiry).Unix()),

		// Information about the service account rule used.
		"config_rev":  i.ConfigRev,
		"config_rule": i.Rule.Name,

		// Information about the request handler environment.
		"peer_ip":         i.PeerIP.String(),
		"service_version": i.Response.ServiceVersion,
		"gae_request_id":  i.RequestID,
		"auth_db_rev":     i.AuthDBRev,
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
	row := i.toBigQueryRow()
	if info.IsDevAppServer(c) {
		blob, err := json.MarshalIndent(row, "", "  ")
		if err != nil {
			return err
		}
		logging.Debugf(c, "BigQuery log row:\n%s", blob)
		return nil
	}
	return oauthTokensLog.Insert(c, bqlog.Entry{Data: row})
}

// FlushOAuthTokensLog sends all buffered logged tokens to BigQuery.
//
// It is fine to call FlushOAuthTokensLog concurrently from multiple request
// handlers, if necessary (it will effectively parallelize the flush).
func FlushOAuthTokensLog(c context.Context) error {
	_, err := oauthTokensLog.Flush(c)
	return err
}
