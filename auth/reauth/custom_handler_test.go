// Copyright 2025 The LUCI Authors.
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

package reauth

import (
	"context"
	"encoding/base64"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestPluginEncode(t *testing.T) {
	t.Parallel()
	body := struct {
		You string `json:"you"`
	}{
		You: "me",
	}
	got, err := pluginEncode(body)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("\x0c\x00\x00\x00" + `{"you":"me"}`)
	assert.That(t, got, should.Match(want))
}

func TestPluginDecode(t *testing.T) {
	t.Parallel()
	d := []byte("\xc6\x00\x00\x00" + `{"type":"sign_helper_reply","code":0,"errorDetail":"","responseData":{"appIdHash":"book","challengeHash":"whatami","keyHandle":"dalian","version":"2","signatureData":"theworld"},"nonexistence":"me"}`)
	got, err := pluginDecode(d)
	if err != nil {
		t.Fatal(err)
	}
	want := &pluginResponse{
		Type:        "sign_helper_reply",
		Code:        0,
		ErrorDetail: "",
		ResponseData: pluginResponseData{
			AppIDHash:     "book",
			ChallengeHash: "whatami",
			KeyHandle:     "dalian",
			Version:       "2",
			SignatureData: "theworld",
		},
	}
	assert.That(t, got, should.Match(want))
}

func TestPluginHandler(t *testing.T) {
	t.Parallel()
	const kh = "dalian"
	const ch = "whatami"
	const facetID = "sartre"
	const sig = "theworld"
	clientData := newClientDataJSON([]byte(ch), facetID)
	resp, err := pluginEncode(&pluginResponse{
		Type: "sign_helper_reply",
		ResponseData: pluginResponseData{
			AppIDHash:     "book",
			ChallengeHash: base64.RawURLEncoding.EncodeToString(sha256hash(clientData)),
			KeyHandle:     base64.RawURLEncoding.EncodeToString([]byte(kh)),
			Version:       "U2F_V2",
			SignatureData: base64.RawURLEncoding.EncodeToString([]byte(sig)),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	p := dummyPlugin{
		resp: resp,
	}
	h := pluginHandler{
		facetID: facetID,
		send:    p.send,
	}
	ctx := context.Background()
	c := challenge{
		SecurityKey: skProposal{
			AppID: "https://www.gstatic.com/securitykey/a/google.com/origins.json",
			DeviceChallenges: []deviceChallenge{{
				KeyHandle: []byte(kh),
				Challenge: []byte(ch),
			}},
		},
	}
	got, err := h.Handle(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	want := &proposalReply{
		SecurityKey: skReply{
			AppID:         "https://www.gstatic.com/securitykey/a/google.com/origins.json",
			ClientData:    clientData,
			SignatureData: []byte(sig),
			KeyHandle:     []byte(kh),
			ReplyType:     "U2F",
		},
	}
	assert.That(t, got, should.Match(want))
	sendWant, err := pluginEncode(&pluginRequest{
		Type: "sign_helper_request",
		SignData: []pluginSignData{{
			AppIDHash:     "ZEZHL99u7Xvzwzcg8jZnbDbhtF6-BIXbiaPN_dJL1p8",
			ChallengeHash: base64.RawURLEncoding.EncodeToString(sha256hash(clientData)),
			KeyHandle:     base64.RawURLEncoding.EncodeToString([]byte(kh)),
			Version:       "U2F_V2",
		}},
		TimeoutSecs: 30,
		LocalAlways: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("send received request: %q", p.got)
	assert.That(t, p.got, should.Match(sendWant))
}

type dummyPlugin struct {
	got  []byte
	resp []byte
	err  error
}

func (p *dummyPlugin) send(ctx context.Context, d []byte) ([]byte, error) {
	p.got = make([]byte, len(d))
	copy(p.got, d)
	return p.resp, p.err
}
