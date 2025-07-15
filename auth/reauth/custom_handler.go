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
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/webauthn"
)

// A customSKHandler supports U2F challenge via an external tool.
//
// The signing plugin should implement the following interface:
//
// Communication occurs over stdin/stdout, and messages are both sent and
// received in the form:
//
//	[4 bytes - payload size (little-endian)][variable bytes - json payload]
//
// The JSON payloads are defined in [webauthn].
type customSKHandler struct {
	pluginHandler
	facetID string
}

func newCustomSKHandler(facetID string) customSKHandler {
	h := customSKHandler{
		pluginHandler: pluginHandler{
			facetID: facetID,
		},
	}
	h.pluginHandler.send = h.send
	return h
}

func (customSKHandler) pluginCmd() string {
	return os.Getenv("GOOGLE_AUTH_WEBAUTHN_PLUGIN")
}

func (h customSKHandler) IsAvailable() bool {
	return h.pluginCmd() != ""
}

// send sends bytes to the plugin command and returns the output.
func (h customSKHandler) send(ctx context.Context, d []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, h.pluginCmd())
	cmd.Stdin = bytes.NewReader(d)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			logging.Debugf(ctx, "signing plugin stderr: %q", exitErr.Stderr)
		}
	}
	return out, err
}

// A pluginHandler implements U2F challenges via a plugin.
//
// The core logic is implemented, with the low level plugin
// interaction abstracted to enable testing.
type pluginHandler struct {
	facetID string
	send    func(context.Context, []byte) ([]byte, error)
}

func (h pluginHandler) Handle(ctx context.Context, c challenge) (*proposalReply, error) {
	logging.Debugf(ctx, "Starting pluginHandler.Handle")
	defer logging.Debugf(ctx, "Exiting pluginHandler.Handle")
	logging.Debugf(ctx, "Got AppID %q", c.SecurityKey.AppID)
	dc := c.SecurityKey.DeviceChallenges
	if len(dc) == 0 {
		return nil, errors.New("pluginHandler: no device challenges available")
	}
	allowed := make([]webauthn.PublicKeyCredentialDescriptor, len(dc))
	for i := range dc {
		id, err := pluginFromReAuth(dc[i].KeyHandle)
		if err != nil {
			return nil, errors.Fmt("pluginHandler: encode key handle: %s", err)
		}
		allowed[i] = webauthn.PublicKeyCredentialDescriptor{
			Type: "public-key",
			ID:   id,
		}
	}

	challenge, err := pluginFromReAuth(dc[0].Challenge)
	if err != nil {
		return nil, errors.Fmt("pluginHandler: encode challenge: %s", err)
	}
	req := &webauthn.GetAssertionRequest{
		Type:   "get",
		Origin: h.facetID,
		RequestData: webauthn.GetAssertionRequestData{
			RPID:             c.SecurityKey.RelyingPartyID,
			Challenge:        challenge,
			TimeoutMillis:    30_000,
			AllowCredentials: allowed,
			UserVerification: "required",
			Extensions: map[string]any{
				"appid": c.SecurityKey.AppID,
			},
		},
	}

	resp, err := h.authWithPlugin(ctx, req)
	if err != nil {
		return nil, errors.Fmt("pluginHandler: %w", err)
	}
	logging.Debugf(ctx, "Got plugin response: %+v", resp)
	if resp.Error != "" {
		return nil, errors.Fmt("pluginHandler: error from plugin: %s", resp.Error)
	}

	reply := skReply{
		ClientData:        resp.ResponseData.Response.ClientData,
		AuthenticatorData: resp.ResponseData.Response.AuthenticatorData,
		SignatureData:     resp.ResponseData.Response.Signature,
		AppID:             c.SecurityKey.AppID,
		KeyHandle:         resp.ResponseData.ID,
		ReplyType:         skWebAuthn,
	}

	return &proposalReply{
		SecurityKey: reply,
	}, nil
}

func (h pluginHandler) authWithPlugin(ctx context.Context, req *webauthn.GetAssertionRequest) (*webauthn.GetAssertionResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	logging.Debugf(ctx, "Attempting signing plugin auth with request: %+v", req)
	fmt.Print("\nYou may need to touch the flashing security key device to proceed.\n\n")
	return h.sendRequest(ctx, req)
}

// sendRequest sends a request to the signing plugin.
//
// Note that this may return both a response and an error.
// In this case, [pluginResponse.Code] may be zero, but it should be
// treated as an error.
// The response may contain more details about the error from the plugin.
func (h pluginHandler) sendRequest(ctx context.Context, req *webauthn.GetAssertionRequest) (*webauthn.GetAssertionResponse, error) {
	ereq, err := pluginEncode(req)
	if err != nil {
		return nil, errors.Fmt("pluginHandler.sendRequest: %w", err)
	}
	logging.Debugf(ctx, "Sending signing plugin input: %q", ereq)
	out, err := h.send(ctx, ereq)
	var resp *webauthn.GetAssertionResponse
	if err := pluginDecode(out, &resp); err != nil {
		logging.Debugf(ctx, "Error unmarshalling plugin response: %q", out)
		return nil, errors.Fmt("pluginHandler.sendRequest: %w", err)
	}
	return resp, nil
}

// pluginHeaderOrder is the byte order for signing plugin message headers.
var pluginHeaderOrder = binary.LittleEndian

// pluginEncode encodes framed JSON messages for signing plugins.
func pluginEncode(v any) ([]byte, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, errors.Fmt("pluginEncode: %w", err)
	}
	// Check body length fits within 32-bit limit of the encoding.
	// Note we cast both LHS and RHS to an int64 to avoid coercing
	// RHS to an int (which may be only 32 bits on some platforms).
	if int64(len(body)) > int64(math.MaxUint32) {
		return nil, errors.New("pluginEncode: body too big")
	}
	// If we are running on a platform where ints are 32 bits
	// or less, check len(body)+4 will not overflow int.
	if len(body) > math.MaxInt-4 {
		return nil, errors.Fmt("pluginEncode: body exceeding %v unsupported on this platform", math.MaxInt-4)
	}
	msg := make([]byte, len(body)+4)
	if _, err = binary.Encode(msg[:4], pluginHeaderOrder, uint32(len(body))); err != nil {
		return nil, errors.Fmt("pluginEncode: %w", err)
	}
	copy(msg[4:], body)
	return msg, nil
}

// pluginDecode decodes framed JSON messages from signing plugins.
func pluginDecode(d []byte, v any) error {
	if len(d) < 4 {
		return errors.New("pluginDecode: input too short")
	}
	var bodyLen uint32
	n, err := binary.Decode(d[:4], pluginHeaderOrder, &bodyLen)
	if err != nil {
		return errors.Fmt("pluginDecode: %w", err)
	}
	if n != 4 {
		panic(fmt.Sprintf("read unexpected number of header bytes %d", n))
	}
	if int64(bodyLen) != int64(len(d)-4) {
		return errors.Fmt("pluginDecide: message declared %d length, but actual length is %d (with 4 bytes header)", bodyLen, len(d))
	}
	if err := json.Unmarshal(d[4:], v); err != nil {
		return errors.Fmt("pluginDecode: %w", err)
	}
	return nil
}

// pluginFromReAuth converts a ReAuth base64 string to a plugin base64 string.
func pluginFromReAuth(s string) (string, error) {
	s2, err := transcodeBase64(webauthn.Base64Encoding, reauthEncoding, s)
	if err != nil {
		return "", errors.Fmt("pluginFromReAuth: %w", err)
	}
	return s2, nil
}

type base64Encoding interface {
	DecodeString(string) ([]byte, error)
	EncodeToString([]byte) string
}

func transcodeBase64(to, from base64Encoding, s string) (string, error) {
	d, err := from.DecodeString(s)
	if err != nil {
		return "", err
	}
	return to.EncodeToString(d), nil
}
