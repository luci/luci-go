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
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"go.chromium.org/luci/common/exec/execmock"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/webauthn"
)

type pluginMockConfig struct {
	// What the plugin mock writes to stdout.
	Output []byte
}
type pluginMockResult struct {
	// What the plugin mock received from stdin.
	Got []byte
}

var pluginExecMock = execmock.Register(func(c pluginMockConfig) (*pluginMockResult, int, error) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, 1, err
	}

	if _, err := os.Stdout.Write(c.Output); err != nil {
		return nil, 1, err
	}

	return &pluginMockResult{Got: data}, 0, nil
})

func TestPluginReadFrame(t *testing.T) {
	t.Parallel()

	t.Run("correctly framed messages", func(t *testing.T) {
		t.Parallel()

		// 4 byte header + 1 byte data.
		frame1 := []byte{0x01, 0x00, 0x00, 0x00, 0xff}

		// 4 byte header + 2 byte data.
		frame2 := []byte{0x02, 0x00, 0x00, 0x00, 0xff, 0xfe}

		// Combine two frames into a single reader.
		r := io.MultiReader(bytes.NewReader(frame1), bytes.NewReader(frame2))

		got1, err := PluginReadFrame(r)
		assert.NoErr(t, err)
		assert.That(t, got1, should.Match([]byte{0xff}))

		got2, err := PluginReadFrame(r)
		assert.NoErr(t, err)
		assert.That(t, got2, should.Match([]byte{0xff, 0xfe}))
	})

	t.Run("incomplete header", func(t *testing.T) {
		t.Parallel()

		frame := []byte{0xff}

		_, err := PluginReadFrame(bytes.NewReader(frame))
		assert.ErrIsLike(t, err, io.ErrUnexpectedEOF)
	})

	t.Run("incomplete body", func(t *testing.T) {
		t.Parallel()

		// 4 byte header (length 255), 1 byte body.
		frame := []byte{0xff, 0x00, 0x00, 0x00, 0x01}

		_, err := PluginReadFrame(bytes.NewReader(frame))
		assert.ErrIsLike(t, err, io.ErrUnexpectedEOF)
	})
}

func TestPluginWriteFrame(t *testing.T) {
	t.Parallel()

	var b bytes.Buffer
	err := PluginWriteFrame(&b, []byte{0xca, 0xfe})
	assert.NoErr(t, err)
	assert.That(t, b.Bytes(), should.Match([]byte{0x02, 0x00, 0x00, 0x00, 0xca, 0xfe}))
}

func TestPluginEncode(t *testing.T) {
	t.Parallel()
	body := struct {
		You string `json:"you"`
	}{
		You: "me",
	}
	got, err := PluginEncode(body)
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
	if err := PluginDecode(d, &got); err != nil {
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
	pluginOutput, err := PluginEncode(&webauthn.GetAssertionResponse{
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

	ctx := execmock.Init(context.Background())
	pluginUses := pluginExecMock.Mock(ctx, pluginMockConfig{Output: pluginOutput})

	h := pluginHandler{facetID: "sartre"}
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
	sendWant, err := PluginEncode(&webauthn.GetAssertionRequest{
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
			UserVerification: "preferred",
			Extensions:       map[string]any{"appid": "google.com"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	pluginInvocations := pluginUses.Snapshot()
	assert.Loosely(t, pluginInvocations, should.HaveLength(1))
	pluginResult, _, err := pluginInvocations[0].GetOutput(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("send received request: %q", pluginResult.Got)
	assert.Loosely(t, pluginResult.Got, should.Match(sendWant))
}

func TestMain(m *testing.M) {
	execmock.Intercept(true)
	os.Exit(m.Run())
}
