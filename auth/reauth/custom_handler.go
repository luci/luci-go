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
	"io"
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
		id, err := PluginFromReAuthBase64(dc[i].KeyHandle)
		if err != nil {
			return nil, errors.Fmt("pluginHandler: encode key handle: %s", err)
		}
		allowed[i] = webauthn.PublicKeyCredentialDescriptor{
			Type: "public-key",
			ID:   id,
		}
	}

	challenge, err := PluginFromReAuthBase64(dc[0].Challenge)
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
	ereq, err := PluginEncode(req)
	if err != nil {
		return nil, errors.Fmt("pluginHandler.sendRequest: %w", err)
	}
	logging.Debugf(ctx, "Sending signing plugin input: %q", ereq)
	out, err := h.send(ctx, ereq) // nolint:ineffassign
	var resp *webauthn.GetAssertionResponse
	if err := PluginDecode(out, &resp); err != nil {
		logging.Debugf(ctx, "Error unmarshalling plugin response: %q", out)
		return nil, errors.Fmt("pluginHandler.sendRequest: %w", err)
	}
	return resp, nil
}

// pluginHeaderOrder is the byte order for signing plugin message headers.
var pluginHeaderOrder = binary.LittleEndian

// PluginReadFrame reads a framed message from `r` and returns the message body.
func PluginReadFrame(r io.Reader) ([]byte, error) {
	var bodyLen uint32

	if err := binary.Read(r, pluginHeaderOrder, &bodyLen); err != nil {
		return nil, errors.WrapIf(err, "pluginRead: failed to read plugin frame header")
	}

	// Check the message body can fit in a []byte. So we don't panic when
	// we allocate the `body` []byte below.
	//
	// For example, on 32-bit systems, the max supported size MaxInt is smaller
	// than what's permitted in the plugin protocol (MaxUint32).
	if int64(bodyLen) > int64(math.MaxInt) {
		return nil, errors.Fmt("pluginRead: plugin frame body size is too large for this platform, got: %v, max supported size: %v", bodyLen, math.MaxInt)
	}

	body := make([]byte, bodyLen)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, errors.WrapIf(err, "pluginRead: failed to read plugin frame body")
	}

	return body, nil
}

// PluginWriteFrame writes `body` to `w` as a framed plugin message for signing plugin.
func PluginWriteFrame(w io.Writer, body []byte) (err error) {
	bodyLen := len(body)

	// Cast to int64 to avoid coercing RHS to int (which might be int32 on certain platforms).
	if int64(bodyLen) > int64(math.MaxUint32) {
		return errors.New("pluginWrite: body too big")
	}

	if err := binary.Write(w, pluginHeaderOrder, uint32(bodyLen)); err != nil {
		return errors.WrapIf(err, "pluginWrite: failed to write plugin frame header")
	}

	if _, err := w.Write(body); err != nil {
		return errors.WrapIf(err, "pluginWrite: failed to write plugin frame body")
	}

	return nil
}

// PluginEncode encodes framed JSON messages for signing plugins.
func PluginEncode(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := PluginEncodeStream(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// PluginEncodeStream encodes framed JSON messages for signing plugins into a stream.
func PluginEncodeStream(w io.Writer, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return errors.Fmt("pluginEncodeStream: %w", err)
	}
	if err := PluginWriteFrame(w, body); err != nil {
		return errors.WrapIf(err, "pluginEncodeStream: failed to encode")
	}
	return nil
}

// PluginDecode decodes framed JSON messages from signing plugins.
func PluginDecode(d []byte, v any) error {
	return PluginDecodeStream(bytes.NewReader(d), v)
}

// PluginDecodeStream decodes a framed JSON message from a signing plugin stream.
func PluginDecodeStream(r io.Reader, v any) error {
	body, err := PluginReadFrame(r)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(body, v); err != nil {
		return errors.Fmt("pluginDecodeStream: %w", err)
	}
	return nil
}
