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
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/webauthn"
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
	d := []byte("\x1e\x00\x00\x00" + `{"type":"get","origin":"seia"}`)
	var got map[string]string
	if err := pluginDecode(d, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"type":   "get",
		"origin": "seia",
	}
	assert.That(t, got, should.Match(want))
}

func TestPluginHandler(t *testing.T) {
	t.Parallel()
	resp, err := pluginEncode(&webauthn.GetAssertionResponse{
		Type: "getResponse",
		ResponseData: webauthn.GetAssertionResponseData{
			Type: "public-key",
			ID:   "dalian-__A",
			Response: webauthn.AuthenticatorAssertionResponse{
				ClientData:        "ronova-_",
				AuthenticatorData: "naberius-_",
				Signature:         "istaroth-_",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	p := dummyPlugin{
		resp: resp,
	}

	h := pluginHandler{
		facetID: "sartre",
		send:    p.send,
	}
	ctx := context.Background()
	c := challenge{
		SecurityKey: skProposal{
			AppID:          "google.com",
			RelyingPartyID: "google.com",
			DeviceChallenges: []deviceChallenge{{
				KeyHandle: "dalian+//A==",
				Challenge: "whatami+/A==",
			}},
		},
	}
	got, err := h.Handle(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	want := &proposalReply{
		SecurityKey: skReply{
			AppID:             "google.com",
			ClientData:        "ronova-_",
			SignatureData:     "istaroth-_",
			KeyHandle:         "dalian-__A",
			AuthenticatorData: "naberius-_",
			ReplyType:         skWebAuthn,
		},
	}
	assert.That(t, got, should.Match(want))
	sendWant, err := pluginEncode(&webauthn.GetAssertionRequest{
		Type:   "get",
		Origin: "sartre",
		RequestData: webauthn.GetAssertionRequestData{
			RPID:          "google.com",
			Challenge:     "whatami-_A",
			TimeoutMillis: 30_000,
			AllowCredentials: []webauthn.PublicKeyCredentialDescriptor{{
				Type: "public-key",
				ID:   "dalian-__A",
			}},
			UserVerification: "required",
			Extensions:       map[string]any{"appid": "google.com"},
		},
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
