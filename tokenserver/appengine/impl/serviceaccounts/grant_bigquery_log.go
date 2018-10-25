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
	"fmt"
	"net"

	"github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/appengine"

	"go.chromium.org/luci/appengine/bqlog"
	"go.chromium.org/luci/common/bq"

	"go.chromium.org/luci/tokenserver/api"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
	bqpb "go.chromium.org/luci/tokenserver/api/bq"
	"go.chromium.org/luci/tokenserver/api/minter/v1"

	"go.chromium.org/luci/tokenserver/appengine/impl/utils"
)

var oauthTokenGrantsLog = bqlog.Log{
	QueueName:           "bqlog-oauth-token-grants", // see queues.yaml
	DatasetID:           "tokens",                   // see bq/README.md
	TableID:             "oauth_token_grants",       // see bq/tables/oauth_token_grants.schema
	DumpEntriesToLogger: true,
	DryRun:              appengine.IsDevAppServer(),
}

// MintedGrantInfo is passed to LogGrant.
//
// It carries all information about the grant minting operation and the produced
// grant token.
type MintedGrantInfo struct {
	Request   *minter.MintOAuthTokenGrantRequest  // RPC input, as is
	Response  *minter.MintOAuthTokenGrantResponse // RPC output, as is
	GrantBody *tokenserver.OAuthTokenGrantBody    // deserialized grant
	ConfigRev string                              // revision of the service_accounts.cfg used
	Rule      *admin.ServiceAccountRule           // the particular rule used to authorize the request
	PeerIP    net.IP                              // caller IP address
	RequestID string                              // GAE request ID that handled the RPC
	AuthDBRev int64                               // revision of groups database (or 0 if unknown)
}

// toBigQueryMessage returns a message to upload to BigQuery.
func (i *MintedGrantInfo) toBigQueryMessage() *bqpb.OAuthTokenGrant {
	return &bqpb.OAuthTokenGrant{
		// Information about the produced token.
		Fingerprint:     utils.TokenFingerprint(i.Response.GrantToken),
		TokenId:         fmt.Sprintf("%d", i.GrantBody.TokenId),
		ServiceAccount:  i.GrantBody.ServiceAccount,
		ProxyIdentity:   i.GrantBody.Proxy,
		EndUserIdentity: i.GrantBody.EndUser,
		IssuedAt:        i.GrantBody.IssuedAt,
		Expiration: &timestamp.Timestamp{
			Seconds: i.GrantBody.IssuedAt.Seconds + i.GrantBody.ValidityDuration,
			Nanos:   i.GrantBody.IssuedAt.Nanos,
		},

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

// LogGrant records information about the OAuth token grant in the BigQuery.
//
// The grant itself is not logged. Only first 16 bytes of its SHA256 hash
// (aka 'fingerprint') is. It is used only to identify this particular token in
// logs.
//
// On dev server, logs to the GAE log only, not to BigQuery (to avoid
// accidentally pushing fake data to real BigQuery dataset).
func LogGrant(c context.Context, i *MintedGrantInfo) error {
	return oauthTokenGrantsLog.Insert(c, &bq.Row{
		Message: i.toBigQueryMessage(),
	})
}

// FlushGrantsLog sends all buffered logged grants to BigQuery.
//
// It is fine to call FlushGrantLog concurrently from multiple request handlers,
// if necessary (it will effectively parallelize the flush).
func FlushGrantsLog(c context.Context) error {
	_, err := oauthTokenGrantsLog.Flush(c)
	return err
}
