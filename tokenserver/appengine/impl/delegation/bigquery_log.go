// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package delegation

import (
	"encoding/json"
	"fmt"
	"net"

	"golang.org/x/net/context"

	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/common/logging"

	"github.com/luci/luci-go/tokenserver/api/admin/v1"
	"github.com/luci/luci-go/tokenserver/api/minter/v1"
	"github.com/luci/luci-go/tokenserver/appengine/impl/utils"
	"github.com/luci/luci-go/tokenserver/appengine/impl/utils/bqlog"
)

var delegationTokensLog = bqlog.Log{
	QueueName: "bqlog-delegation-tokens", // see queues.yaml
	DatasetID: "tokens",                  // see bq/README.md
	TableID:   "delegation_tokens",       // see bq/tables/delegation_tokens.schema
}

// MintedTokenInfo is passed to LogToken.
//
// It carries all information about the token minting operation and the produced
// token.
type MintedTokenInfo struct {
	Request   *minter.MintDelegationTokenRequest  // RPC input, as is
	Response  *minter.MintDelegationTokenResponse // RPC output, as is
	Config    *DelegationConfig                   // the delegation config used
	Rule      *admin.DelegationRule               // the delegation rule used to authorize the request
	PeerIP    net.IP                              // caller IP address
	RequestID string                              // GAE request ID that handled the RPC
}

// toBigQueryRow returns a JSON-ish map to upload to BigQuery.
//
// It's schema must match 'bq/tables/delegation_tokens.schema'.
func (i *MintedTokenInfo) toBigQueryRow() map[string]interface{} {
	subtok := i.Response.DelegationSubtoken
	return map[string]interface{}{
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
		"config_rev":  i.Config.Revision,
		"config_rule": i.Rule.Name,

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
	row := i.toBigQueryRow()
	if info.IsDevAppServer(c) {
		blob, err := json.MarshalIndent(row, "", "  ")
		if err != nil {
			return err
		}
		logging.Debugf(c, "BigQuery log row:\n%s", blob)
		return nil
	}
	return delegationTokensLog.Insert(c, bqlog.Entry{Data: row})
}

// FlushTokenLog sends all buffered logged tokens to BigQuery.
//
// It is fine to call FlushTokenLog concurrently from multiple request handlers,
// if necessary (it will effectively parallelize the flush).
func FlushTokenLog(c context.Context) error {
	_, err := delegationTokensLog.Flush(c)
	return err
}
