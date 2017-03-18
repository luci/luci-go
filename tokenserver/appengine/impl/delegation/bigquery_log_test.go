// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package delegation

import (
	"net"
	"testing"

	"github.com/luci/luci-go/server/auth/delegation/messages"
	"github.com/luci/luci-go/tokenserver/api/admin/v1"
	"github.com/luci/luci-go/tokenserver/api/minter/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMintedTokenInfo(t *testing.T) {
	Convey("produces correct row map", t, func() {
		info := MintedTokenInfo{
			Request: &minter.MintDelegationTokenRequest{
				ValidityDuration: 3600,
				Intent:           "intent string",
			},
			Response: &minter.MintDelegationTokenResponse{
				Token:          "blah",
				ServiceVersion: "unit-tests/mocked-ver",
				DelegationSubtoken: &messages.Subtoken{
					Kind:              messages.Subtoken_BEARER_DELEGATION_TOKEN,
					SubtokenId:        1234,
					DelegatedIdentity: "user:delegated@example.com",
					RequestorIdentity: "user:requestor@example.com",
					CreationTime:      1422936306,
					ValidityDuration:  3600,
					Audience:          []string{"user:audience@example.com"},
					Services:          []string{"*"},
				},
			},
			Config: &DelegationConfig{
				Revision: "config-rev",
			},
			Rule: &admin.DelegationRule{
				Name: "rule-name",
			},
			PeerIP:    net.ParseIP("127.10.10.10"),
			RequestID: "gae-request-id",
		}

		So(info.toBigQueryRow(), ShouldResemble, map[string]interface{}{
			"config_rev":         "config-rev",
			"config_rule":        "rule-name",
			"delegated_identity": "user:delegated@example.com",
			"expiration":         1.422939906e+09,
			"fingerprint":        "8b7df143d91c716ecfa5fc1730022f6b",
			"gae_request_id":     "gae-request-id",
			"issued_at":          1.422936306e+09,
			"peer_ip":            "127.10.10.10",
			"requested_intent":   "intent string",
			"requested_validity": int(3600), // convey is pedantic about types
			"requestor_identity": "user:requestor@example.com",
			"service_version":    "unit-tests/mocked-ver",
			"target_audience":    []string{"user:audience@example.com"},
			"target_services":    []string{"*"},
			"token_id":           "1234",
			"token_kind":         "BEARER_DELEGATION_TOKEN",
		})
	})
}
