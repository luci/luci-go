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

// Package reauth implements ReAuth API support.
package reauth

import (
	"context"
	"net/http"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

const (
	tokenURI     = "https://oauth2.googleapis.com/token"
	reAuthOrigin = "https://accounts.google.com"

	runChallengeLimit = 5
)

// A RAPT is a ReAuth proof token.
type RAPT struct {
	Token  string    `json:"token"`
	Expiry time.Time `json:"expiry"`
}

// GetRAPT performs a ReAuth flow and returns the proof token.
//
// The HTTP client should be authenticated with the ReAuth scope.
// This should always be assumed to require user interaction.
func GetRAPT(ctx context.Context, c *http.Client) (*RAPT, error) {
	return getRAPT(ctx, c, challengeHandlers(reAuthOrigin))
}

// getRAPT performs a ReAuth flow and returns the proof token.
//
// The HTTP client should be authenticated with the ReAuth scope.
// This should always be assumed to require user interaction.
//
// Takes a map of [challengeHandlers], for handling ReAuth challenges,
// naturally.
func getRAPT(ctx context.Context, c *http.Client, h map[string]challengeHandler) (*RAPT, error) {
	logging.Debugf(ctx, "Starting ReAuth session...")
	sr, err := startSession(ctx, c)
	if err != nil {
		return nil, errors.Fmt("GetRAPT: %w", err)
	}
	for range runChallengeLimit {
		logging.Debugf(ctx, "ReAuth session response: %+v", sr)
		switch r := sr.RejectionReason; r {
		case "REAUTH_REJECTION_REASON_UNSPECIFIED", "":
		default:
			return nil, errors.Fmt("GetRAPT: reauth rejected with %q", r)
		}
		switch s := sr.Status; s {
		case "AUTHENTICATED":
			return &RAPT{
				Token:  sr.EncodedRAPT,
				Expiry: time.Now().Add(ProofTokenLifetime),
			}, nil
		case "CHALLENGE_REQUIRED", "CHALLENGE_PENDING":
		default:
			return nil, errors.Fmt("GetRAPT: unexpected reauth status %q", s)
		}

		ch, ok := nextReadyChallenge(sr.Challenges)
		if !ok {
			return nil, errors.New("GetRAPT: no ready challenges")
		}
		h, ok := h[ch.ChallengeType]
		if !ok {
			return nil, errors.Fmt("GetRAPT: unsupported challenge type %q", ch.ChallengeType)
		}
		if err := h.CheckAvailable(ctx); err != nil {
			return nil, errors.Fmt("GetRAPT: handler %T reports itself not available: %v", h, err)
		}
		pr, err := h.Handle(ctx, ch)
		if err != nil {
			return nil, errors.Fmt("GetRAPT: %w", err)
		}
		req := &continueRequest{
			Action:      "RESPOND",
			ChallengeID: ch.ChallengeID,
			Response:    *pr,
		}
		logging.Debugf(ctx, "Sending ReAuth continue session request %+v", req)
		sr, err = continueSession(ctx, c, sr.SessionID, req)
		if err != nil {
			return nil, errors.Fmt("GetRAPT: %w", err)
		}
	}
	return nil, errors.New("GetRAPT: exceeded challenge limit")
}

func nextReadyChallenge(ch []challenge) (challenge, bool) {
	for _, ch := range ch {
		if ch.Status != "READY" && ch.Status != "RETRYABLE_ERROR" {
			continue
		}
		return ch, true
	}
	return challenge{}, false
}
