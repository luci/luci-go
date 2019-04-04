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

	"github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/appengine"

	"go.chromium.org/luci/appengine/bqlog"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/bq"

	bqpb "go.chromium.org/luci/tokenserver/api/bq"
	"go.chromium.org/luci/tokenserver/api/minter/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils"
)

var projectTokensLog = bqlog.Log{
	QueueName:           "bqlog-project-tokens", // see queues.yaml
	DatasetID:           "tokens",               // see bq/README.md
	TableID:             "project_tokens",       // see bq/tables/project_tokens.schema
	DumpEntriesToLogger: true,
	DryRun:              appengine.IsDevAppServer(),
}

// MintedTokenInfo is passed to LogToken.
//
// It carries all information about the token minting operation and the produced
// token.
type MintedTokenInfo struct {
	Request      *minter.MintProjectTokenRequest  // RPC input, as is
	Response     *minter.MintProjectTokenResponse // RPC output, as is
	RequestedAt  *timestamp.Timestamp
	Expiration   *timestamp.Timestamp
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

// LogToken records information about the token in the BigQuery.
//
// The signed token itself is not logged. Only first 16 bytes of its SHA256 hash
// (aka 'fingerprint') is. It is used only to identify this particular token in
// logs.
//
// On dev server, logs to the GAE log only, not to BigQuery (to avoid
// accidentally pushing fake data to real BigQuery dataset).
func LogToken(c context.Context, i *MintedTokenInfo) error {
	return projectTokensLog.Insert(c, &bq.Row{
		Message: i.toBigQueryMessage(),
	})
}

// FlushTokenLog sends all buffered logged tokens to BigQuery.
//
// It is fine to call FlushTokenLog concurrently from multiple request handlers,
// if necessary (it will effectively parallelize the flush).
func FlushTokenLog(c context.Context) error {
	_, err := projectTokensLog.Flush(c)
	return err
}
