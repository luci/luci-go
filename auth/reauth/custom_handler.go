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
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"sync"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// pluginEncoding is the base64 encoding used by signing plugins.
var pluginEncoding = base64.RawURLEncoding

// A customSKHandler supports U2F challenge via an external tool.
//
// The signing plugin should implement the following interface:
//
// Communication occurs over stdin/stdout, and messages are both sent and
// received in the form:
//
//	[4 bytes - payload size (little-endian)][variable bytes - json payload]
//
// Signing Request JSON
//
//	{
//	  "type": "sign_helper_request",
//	  "signData": [{
//	      "keyHandle": <url-safe base64-encoded key handle>,
//	      "appIdHash": <url-safe base64-encoded SHA-256 hash of application ID>,
//	      "challengeHash": <url-safe base64-encoded SHA-256 hash of ClientData>,
//	      "version": U2F protocol version (usually "U2F_V2")
//	      },...],
//	  "timeoutSeconds": <security key touch timeout>
//	}
//
// Signing Response JSON
//
//	{
//	  "type": "sign_helper_reply",
//	  "code": <result code>.
//	  "errorDetail": <text description of error>,
//	  "responseData": {
//	    "appIdHash": <url-safe base64-encoded SHA-256 hash of application ID>,
//	    "challengeHash": <url-safe base64-encoded SHA-256 hash of ClientData>,
//	    "keyHandle": <url-safe base64-encoded key handle>,
//	    "version": <U2F protocol version>,
//	    "signatureData": <url-safe base64-encoded signature>
//	  }
//	}
//
// Possible response error codes are:
//
//	NoError            = 0
//	UnknownError       = -127
//	TouchRequired      = 0x6985
//	WrongData          = 0x6a80
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
	return os.Getenv("SK_SIGNING_PLUGIN")
}

func (h customSKHandler) IsAvailable() bool {
	return h.pluginCmd() != ""
}

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
	if id := c.SecurityKey.AppID; id != reauthAppID {
		return nil, errors.Reason("pluginHandler: bad AppId %q", id).Err()
	}
	dc := c.SecurityKey.DeviceChallenges
	if len(dc) == 0 {
		return nil, errors.Reason("pluginHandler: no device challenges available").Err()
	}
	clientData := make([][]byte, len(dc))
	signData := make([]pluginSignData, len(dc))
	appIDHash := pluginEncoding.EncodeToString(sha256hash(c.SecurityKey.AppID))
	for i := range dc {
		clientData[i] = newClientDataJSON(dc[i].Challenge, h.facetID)
		signData[i] = pluginSignData{
			AppIDHash:     appIDHash,
			ChallengeHash: pluginEncoding.EncodeToString(sha256hash(clientData[i])),
			KeyHandle:     pluginEncoding.EncodeToString(dc[i].KeyHandle),
			Version:       "U2F_V2",
		}
	}
	req := &pluginRequest{
		Type:        "sign_helper_request",
		SignData:    signData,
		TimeoutSecs: 30,
		LocalAlways: true,
	}
	var resp *pluginResponse
	resp, err := h.authWithPlugin(ctx, req)
	if err != nil {
		return nil, errors.Annotate(err, "pluginHandler").Err()
	}
	rd := resp.ResponseData
	for i := range signData {
		if signData[i].KeyHandle != rd.KeyHandle || signData[i].ChallengeHash != rd.ChallengeHash {
			continue
		}
		var signatureData []byte
		var keyHandle []byte
		signatureData, err = lenientDecodeString(pluginEncoding, rd.SignatureData)
		if err != nil {
			logging.Debugf(ctx, "Error decoding signatureData.  Value=%q", rd.SignatureData)
			return nil, errors.Annotate(err, "pluginHandler: decode response: decode signatureData").Err()
		}
		keyHandle, err = lenientDecodeString(pluginEncoding, rd.KeyHandle)
		if err != nil {
			logging.Debugf(ctx, "Error decoding keyHandle.  Value=%q", rd.KeyHandle)
			return nil, errors.Annotate(err, "pluginHandler: decode response: decode keyHandle").Err()
		}
		return &proposalReply{
			SecurityKey: skReply{
				AppID:         c.SecurityKey.AppID,
				ClientData:    clientData[i],
				SignatureData: signatureData,
				KeyHandle:     keyHandle,
				ReplyType:     "U2F",
			},
		}, nil
	}
	return nil, errors.Reason("pluginHandler: could not find request associated with KeyHandle %q and ChallengeHash %q", rd.KeyHandle, rd.ChallengeHash).Err()
}

func (h pluginHandler) authWithPlugin(ctx context.Context, req *pluginRequest) (*pluginResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	logging.Debugf(ctx, "Attempting signing plugin auth with request: %+v", req)
	prompt := sync.OnceFunc(func() {
		fmt.Print("\nYou may need to touch the flashing security key device to proceed.\n\n")
	})

	// We can't rely on the plugin to be asynchronous, so the
	// plugin request may synchronously hang waiting for a touch.
	// The plugin may also not print any prompt for the user, so
	// we should do it ourselves.
	var wg sync.WaitGroup
	defer wg.Wait()
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()
	t := time.NewTimer(3 * time.Second)
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-t.C:
			prompt()
		case <-ctx.Done():
		}
	}()

	interval := time.NewTicker(250 * time.Millisecond)
	defer interval.Stop()
	for {
		resp, err := h.sendRequest(ctx, req)
		if err != nil {
			logging.Warningf(ctx, "Aborting signing plugin auth due to unknown error: %s", err)
			if resp != nil {
				logging.Warningf(ctx, "Signing plugin returned error details: %s", resp.ErrorDetail)
			}
			return nil, errors.Annotate(err, "signing plugin auth").Err()
		}
		if resp.Code == touchRequired {
			prompt()
			goto cont
		}
		if resp.Code != noError {
			logging.Warningf(ctx, "Aborting signing plugin auth due to unknown error: %s", resp.Code)
			return nil, errors.Annotate(resp.Code, "signing plugin auth").Err()
		}
		logging.Infof(ctx, "Got successful signing plugin response")
		return resp, nil
	cont:
		select {
		case <-ctx.Done():
			return nil, errors.Annotate(ctx.Err(), "signing plugin auth").Err()
		case <-interval.C:
		}
	}
}

// sendRequest sends a request to the signing plugin.
//
// Note that this may return both a response and an error.
// In this case, [pluginResponse.Code] may be zero, but it should be
// treated as an error.
// The response may contain more details about the error from the plugin.
func (h pluginHandler) sendRequest(ctx context.Context, req *pluginRequest) (*pluginResponse, error) {
	ereq, err := pluginEncode(req)
	if err != nil {
		return nil, errors.Annotate(err, "pluginHandler.sendRequest").Err()
	}
	logging.Debugf(ctx, "Sending signing plugin input: %q", ereq)
	out, cmdErr := h.send(ctx, ereq)
	// The signing plugin may exit non-zero, but return a
	// well-formed response that contains the error message, so we
	// juggle the errors a bit.
	resp, err := pluginDecode(out)
	if err != nil {
		if cmdErr != nil {
			return resp, errors.Annotate(cmdErr, "pluginHandler.sendRequest").Err()
		}
		return nil, errors.Annotate(err, "pluginHandler.sendRequest").Err()
	}
	if cmdErr != nil {
		return resp, errors.Annotate(cmdErr, "pluginHandler.sendRequest").Err()
	}
	return resp, nil
}

// A pluginRequest is a request to a signing plugin.
type pluginRequest struct {
	Type        string           `json:"type"`
	SignData    []pluginSignData `json:"signData"`
	TimeoutSecs int              `json:"timeoutSeconds"`

	// Implementation specific fields
	LocalAlways bool `json:"localAlways"`
}

type pluginSignData struct {
	AppIDHash     string `json:"appIdHash"`
	ChallengeHash string `json:"challengeHash"`
	KeyHandle     string `json:"keyHandle"`
	Version       string `json:"version"`
}

// A pluginResponse is a response from a signing plugin.
type pluginResponse struct {
	Type         string             `json:"type"`
	Code         pluginResultCode   `json:"code"`
	ErrorDetail  string             `json:"errorDetail"`
	ResponseData pluginResponseData `json:"responseData"`
}

type pluginResultCode int

const (
	noError       pluginResultCode = 0
	touchRequired pluginResultCode = 0x6985
)

func (c pluginResultCode) Error() string {
	if c <= 0 {
		return fmt.Sprintf("signing plugin result code %d", c)
	}
	return fmt.Sprintf("signing plugin result code %x", int(c))
}

type pluginResponseData struct {
	AppIDHash     string `json:"appIdHash"`
	ChallengeHash string `json:"challengeHash"`
	KeyHandle     string `json:"keyHandle"`
	Version       string `json:"version"`
	SignatureData string `json:"signatureData"`
}

type pluginClientData struct {
	Type      string `json:"typ"`
	Challenge string `json:"challenge"`
	Origin    string `json:"origin"`
}

func newClientDataJSON(challenge []byte, origin string) []byte {
	d, err := json.Marshal(pluginClientData{
		Type:      "navigator.id.getAssertion",
		Challenge: pluginEncoding.EncodeToString(challenge),
		Origin:    origin,
	})
	if err != nil {
		panic(err)
	}
	return d
}

// pluginHeaderOrder is the byte order for signing plugin message headers.
var pluginHeaderOrder = binary.LittleEndian

// pluginEncode encodes framed JSON messages for signing plugins.
func pluginEncode(v any) ([]byte, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, errors.Annotate(err, "pluginEncode").Err()
	}
	if len(body) > math.MaxUint32 {
		return nil, errors.Reason("pluginEncode: body too big").Err()
	}
	msg := make([]byte, len(body)+4)
	if _, err = binary.Encode(msg[:4], pluginHeaderOrder, uint32(len(body))); err != nil {
		return nil, errors.Annotate(err, "pluginEncode").Err()
	}
	copy(msg[4:], body)
	return msg, nil
}

// pluginDecode decodes framed JSON messages from signing plugins.
func pluginDecode(d []byte) (*pluginResponse, error) {
	if len(d) < 4 {
		return nil, errors.Reason("pluginDecode: input too short").Err()
	}
	var bodyLen uint32
	n, err := binary.Decode(d[:4], pluginHeaderOrder, &bodyLen)
	if err != nil {
		return nil, errors.Annotate(err, "pluginDecode").Err()
	}
	if n != 4 {
		panic(fmt.Sprintf("read unexpected number of header bytes %d", n))
	}
	if int(bodyLen) != len(d)-4 {
		return nil, errors.Reason("pluginDecide: message declared %d length, but actual length is %d (with 4 bytes header)", bodyLen, len(d)).Err()
	}
	var resp pluginResponse
	if err := json.Unmarshal(d[4:], &resp); err != nil {
		return nil, errors.Annotate(err, "pluginDecode").Err()
	}
	return &resp, nil
}

// lenientDecodeString decodes base64 with lenient padding.
//
// Specifically, it retries with = padding if initial decode fails.
func lenientDecodeString(e *base64.Encoding, s string) ([]byte, error) {
	d, err := e.DecodeString(s)
	if err == nil {
		return d, nil
	}
	return e.WithPadding(base64.StdPadding).DecodeString(s)
}

func sha256hash[T ~string | ~[]byte](s T) []byte {
	h := sha256.New()
	h.Write([]byte(s))
	return h.Sum(nil)
}
