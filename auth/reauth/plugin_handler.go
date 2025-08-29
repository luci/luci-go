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
	"path"
	"strings"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/exec"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/webauthn"
)

const (
	envPluginCmd   = "GOOGLE_AUTH_WEBAUTHN_PLUGIN"
	envSSHAuthSock = "SSH_AUTH_SOCK"

	// Executable names of the plugins we ship to users.
	sshPluginExecutable   = "luci-auth-ssh-plugin"
	fido2PluginExecutable = "luci-auth-fido2-plugin"
)

// Returns whether the current running environment is an SSH session by probing
// for SSH session environment variables.
func isInSSHSession() bool {
	return os.Getenv("SSH_CONNECTION") != ""
}

// A pluginHandler implements WebAuthn challenges via an external plugin.
//
// The signing plugin should implement the following interface:
//
// Communication occurs over stdin/stdout, and messages are both sent and
// received in the form:
//
//	[4 bytes - payload size (little-endian)][variable bytes - json payload]
//
// The JSON payloads are defined in [webauthn].
type pluginHandler struct {
	facetID string
}

func (pluginHandler) pluginCmd() string {
	// Use plugin environment variable if it's set explicitly.
	if pluginCmd, ok := os.LookupEnv(envPluginCmd); ok {
		return pluginCmd
	}

	// If we're running from a SSH session, use the SSH plugin.
	if isInSSHSession() {
		return sshPluginExecutable
	}

	// Otherwise, default to FIDO2 plugin (which works locally or inside remote
	// desktop sessions).
	return fido2PluginExecutable
}

// Returns an error if continuing ReAuth will fail (i.e. there's an obvious
// misconfiguration).
func (h pluginHandler) CheckAvailable(ctx context.Context) error {
	cmd := h.pluginCmd()
	if cmd == "" {
		return errors.New("security key plugin isn't set")
	}

	logging.Debugf(ctx, "customSKHandler pluginCmd: %v", cmd)
	cmdName := strings.TrimSuffix(path.Base(cmd), path.Ext(cmd))

	// Using SSH plugin.
	if strings.EqualFold(cmdName, sshPluginExecutable) {
		// Error if SSH_AUTH_SOCK isn't set.
		if sshAuthSock := os.Getenv(envSSHAuthSock); sshAuthSock == "" {
			return errors.Fmt("SSH plugin requested, but %s isn't set", envSSHAuthSock)
		}

		// TODO(b/440418327): Dial SSH_AUTH_SOCK and probe the agent is running.
	}

	return nil
}

// send sends bytes to the plugin command and returns the output.
func (h pluginHandler) send(ctx context.Context, d []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, h.pluginCmd())
	cmd.Stdin = bytes.NewReader(d)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	logging.Debugf(ctx, "signing plugin stderr: %q", stderr.Bytes())
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			logging.Debugf(ctx, "signing plugin exit code: %v", exitErr.ExitCode())
			logging.Errorf(ctx, "signing plugin stderr: %q", stderr.Bytes())
		}
	}
	return out, err
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
			UserVerification: "preferred",
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
	h.printForUser("\nStarting plugin (this may take a while the first time)...\n")
	h.printForUser("When your security key starts flashing, touch it to proceed.\n\n")
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
	out, err := h.send(ctx, ereq)
	if err != nil {
		return nil, errors.Fmt("pluginHandler.sendRequest: %w", err)
	}
	var resp *webauthn.GetAssertionResponse
	if err := PluginDecode(out, &resp); err != nil {
		logging.Debugf(ctx, "Error unmarshalling plugin response: %q", out)
		return nil, errors.Fmt("pluginHandler.sendRequest: %w", err)
	}
	return resp, nil
}

// printForUser prints some notice for the user.
func (pluginHandler) printForUser(format string, args ...any) {
	fmt.Printf(format, args...)
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
