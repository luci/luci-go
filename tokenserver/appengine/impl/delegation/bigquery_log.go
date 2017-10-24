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
	"fmt"
	"net"

	"golang.org/x/net/context"
	"google.golang.org/appengine"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/api/minter/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/bqlog"
)

var delegationTokensLog = bqlog.Log{
	QueueName:           "bqlog-delegation-tokens", // see queues.yaml
	DatasetID:           "tokens",                  // see bq/README.md
	TableID:             "delegation_tokens",       // see bq/tables/delegation_tokens.schema
	DumpEntriesToLogger: true,
	DryRun:              appengine.IsDevAppServer(),
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

// toBigQueryRow returns a JSON-ish map to upload to BigQuery.
//
// Its schema must match 'bq/tables/delegation_tokens.schema'.
func (i *MintedTokenInfo) toBigQueryRow() map[string]interface{} {
	subtok := i.Response.DelegationSubtoken

	row := map[string]interface{}{
		// Information about the produced token.
		"fingerprint":        utils.TokenFingerprint(i.Response.Token),
		"token_kind":         subtok.Kind.String(),
		"token_id":           fmt.Sprintf("%d", subtok.SubtokenId),
		"delegated_identity": subtok.DelegatedIdentity,
		"requestor_identity": subtok.RequestorIdentity,
		"issued_at":          float64(subtok.CreationTime),
		"expiration":         float64(subtok.CreationTime + int64(subtok.ValidityDuration)),
		"target_audience":    subtok.Audience,
		"target_services":    subtok.Services,

		// Information about the request.
		"requested_validity": int(i.Request.ValidityDuration),
		"requested_intent":   i.Request.Intent,

		// Information about the delegation rule used.
		"config_rev":  i.ConfigRev,
		"config_rule": i.Rule.Name,

		// Information about the request handler environment.
		"peer_ip":         i.PeerIP.String(),
		"service_version": i.Response.ServiceVersion,
		"gae_request_id":  i.RequestID,
		"auth_db_rev":     i.AuthDBRev,
	}

	// Bigquery doesn't like empty lists or nulls. Omit the column completely.
	if len(subtok.Tags) != 0 {
		row["tags"] = subtok.Tags
	}

	return row
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
	return delegationTokensLog.Insert(c, bqlog.Entry{Data: i.toBigQueryRow()})
}

// FlushTokenLog sends all buffered logged tokens to BigQuery.
//
// It is fine to call FlushTokenLog concurrently from multiple request handlers,
// if necessary (it will effectively parallelize the flush).
func FlushTokenLog(c context.Context) error {
	_, err := delegationTokensLog.Flush(c)
	return err
}
