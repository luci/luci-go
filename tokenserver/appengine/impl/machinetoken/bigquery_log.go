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
	"fmt"
	"net"

	"google.golang.org/appengine"

	"go.chromium.org/luci/tokenserver/api"
	"go.chromium.org/luci/tokenserver/api/minter/v1"

	"go.chromium.org/luci/tokenserver/appengine/impl/certconfig"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/bqlog"
)

var machineTokensLog = bqlog.Log{
	QueueName:           "bqlog-machine-tokens", // see queues.yaml
	DatasetID:           "tokens",               // see bq/README.md
	TableID:             "machine_tokens",       // see bq/tables/machine_tokens.schema
	DumpEntriesToLogger: true,
	DryRun:              appengine.IsDevAppServer(),
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

// toBigQueryRow returns a JSON-ish map to upload to BigQuery.
//
// Its schema must match 'bq/tables/machine_tokens.schema'.
func (i *MintedTokenInfo) toBigQueryRow() map[string]interface{} {
	// LUCI_MACHINE_TOKEN is the only supported type currently.
	if i.Request.TokenType != tokenserver.MachineTokenType_LUCI_MACHINE_TOKEN {
		panic("unknown token type")
	}

	return map[string]interface{}{
		// Identifier of the token body.
		"fingerprint": utils.TokenFingerprint(i.Response.GetLuciMachineToken().MachineToken),

		// Information about the token.
		"machine_fqdn":        i.TokenBody.MachineFqdn,
		"token_type":          i.Request.TokenType.String(),
		"issued_at":           float64(i.TokenBody.IssuedAt),
		"expiration":          float64(i.TokenBody.IssuedAt + i.TokenBody.Lifetime),
		"cert_serial_number":  fmt.Sprintf("%d", i.TokenBody.CertSn),
		"signature_algorithm": i.Request.SignatureAlgorithm.String(),

		// Information about the CA used to authorize this request.
		"ca_common_name": i.CA.CN,
		"ca_config_rev":  i.CA.UpdatedRev,

		// Information about the request handler.
		"peer_ip":         i.PeerIP.String(),
		"service_version": i.Response.ServiceVersion,
		"gae_request_id":  i.RequestID,
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
	return machineTokensLog.Insert(c, bqlog.Entry{Data: i.toBigQueryRow()})
}

// FlushTokenLog sends all buffered logged tokens to BigQuery.
//
// It is fine to call FlushTokenLog concurrently from multiple request handlers,
// if necessary (it will effectively parallelize the flush).
func FlushTokenLog(c context.Context) error {
	_, err := machineTokensLog.Flush(c)
	return err
}
