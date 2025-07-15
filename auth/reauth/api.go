// Copyright 2024 The LUCI Authors.
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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

const (
	ProofTokenLifetime = 20 * time.Hour

	sessionAPI               = "https://reauth.googleapis.com/v2/sessions"
	startSessionURI          = sessionAPI + ":start"
	continueSessionURIFormat = sessionAPI + "/%s:continue"
)

type method int32

const (
	method_SECURE_DEVICE_USER_PRESENCE method = 4 //nolint:stylecheck
)

type reauthConfig struct {
	Method                []method `json:"method"`
	ProofTokenLifetimeSec int64    `json:"proofTokenLifetimeSec"`
}

type startSessionRequest struct {
	SupportedChallengeTypes []string     `json:"supportedChallengeTypes"`
	ReauthConfig            reauthConfig `json:"reauthConfig"`
}

var startSessionBody = sync.OnceValue(func() string {
	b := startSessionRequest{
		SupportedChallengeTypes: []string{"SECURITY_KEY"},
		ReauthConfig: reauthConfig{
			Method:                []method{method_SECURE_DEVICE_USER_PRESENCE},
			ProofTokenLifetimeSec: int64(ProofTokenLifetime / time.Second),
		},
	}
	d, err := json.Marshal(b)
	if err != nil {
		// This should only happen due to coding errors.
		panic(err)
	}
	return string(d)
})

func startSession(ctx context.Context, c *http.Client) (*sessionResponse, error) {
	logging.Debugf(ctx, "Sending ReAuth start session request")
	req, err := http.NewRequestWithContext(ctx, "POST", startSessionURI, strings.NewReader(startSessionBody()))
	if err != nil {
		// This should only happen due to coding errors.
		panic(err)
	}
	req.Header.Add("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return nil, errors.Fmt("start ReAuth session: %w", err)
	}
	sr, err := handleSessionResponse(ctx, resp)
	if err != nil {
		return nil, errors.Fmt("start ReAuth session: %w", err)
	}
	return sr, nil
}

func handleSessionResponse(ctx context.Context, resp *http.Response) (*sessionResponse, error) {
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			logging.Debugf(ctx, "Error reading ReAuth session response body: %v", err)
		}
		logging.Debugf(ctx, "Got ReAuth session response body: %q", string(body))
		return nil, errors.Fmt("unexpected HTTP status %d", resp.StatusCode)
	}
	content, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if content != "application/json" {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			logging.Debugf(ctx, "Error reading ReAuth session response body: %v", err)
		}
		logging.Debugf(ctx, "Got ReAuth session response body: %q", string(body))
		return nil, errors.Fmt("unexpected response content type %q", content)
	}
	var sr sessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, err
	}
	return &sr, nil
}

type sessionResponse struct {
	Status          string      `json:"status"`
	SessionID       string      `json:"sessionId"`
	EncodedRAPT     string      `json:"encodedProofOfReauthToken"`
	Challenges      []challenge `json:"challenges"`
	RejectionReason string      `json:"reauthRejectionReason"`
}

type challenge struct {
	Status        string     `json:"status"`
	ChallengeID   int        `json:"challengeId"`
	ChallengeType string     `json:"challengeType"`
	SecurityKey   skProposal `json:"securityKey"`
}

type skProposal struct {
	AppID            string            `json:"applicationId"`
	RelyingPartyID   string            `json:"relyingPartyId"`
	DeviceChallenges []deviceChallenge `json:"challenges"`
}

type deviceChallenge struct {
	KeyHandle string `json:"keyHandle"`
	Challenge string `json:"challenge"`
}

func continueSession(ctx context.Context, c *http.Client, sessionID string, req *continueRequest) (*sessionResponse, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Fmt("continue ReAuth session: %w", err)
	}
	uri := fmt.Sprintf(continueSessionURIFormat, sessionID)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", uri, bytes.NewReader(b))
	if err != nil {
		// This should only happen due to coding errors.
		panic(err)
	}
	httpReq.Header.Add("Content-Type", "application/json")
	resp, err := c.Do(httpReq)
	if err != nil {
		return nil, errors.Fmt("continue ReAuth session: %w", err)
	}
	sr, err := handleSessionResponse(ctx, resp)
	if err != nil {
		return nil, errors.Fmt("start ReAuth session: %w", err)
	}
	return sr, nil
}

type continueRequest struct {
	Action      string        `json:"action"`
	ChallengeID int           `json:"challengeId"`
	Response    proposalReply `json:"proposalResponse"`
}

type proposalReply struct {
	Credential  string  `json:"credential"`
	SecurityKey skReply `json:"securityKey"`
	// We don't need to support this right now.
	Passkey any `json:"passkey"`
}

type skReply struct {
	AppID             string `json:"applicationId"`
	ClientData        string `json:"clientData"`
	SignatureData     string `json:"signatureData"`
	KeyHandle         string `json:"keyHandle"`
	ReplyType         any    `json:"securityKeyReplyType"`
	AuthenticatorData string `json:"authenticatorData"`
}

const (
	// Values for [skReply.ReplyType].
	skU2F      = "U2F"
	skWebAuthn = 2
)

// base64 encoding used by ReAuth.
var reauthEncoding = base64.StdEncoding
