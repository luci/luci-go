// Copyright 2022 The LUCI Authors.
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

// Package botsrv knows how to authenticate calls from Swarming RBE bots.
//
// It checks PollState tokens and bot credentials.
package botsrv

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"

	internalspb "go.chromium.org/luci/swarming/proto/internals"
)

// Handler handles an authenticated request from a bot.
//
// It returns a response that will be serialized and sent to the bot as JSON or
// a gRPC error code that will be converted into an HTTP error.
type Handler func(ctx context.Context, r *Request) (Response, error)

// Request is an authenticated request from a bot.
type Request struct {
	Body      *RequestBody           // the parsed request body
	PollState *internalspb.PollState // validated deserialized poll token
}

// RequestBody is a JSON structure of the bot request payload.
type RequestBody struct {
	// Dimensions is dimensions reported by the bot.
	Dimensions map[string][]string `json:"dimensions"`
	// State is the state reported by the bot.
	State map[string]interface{} `json:"state"`
	// Version is the bot version.
	Version string `json:"version"`
	// RBEState is RBE-related state reported by the bot.
	RBEState RBEState `json:"rbe_state"`
}

// RBEState is RBE-related state reported by the bot.
type RBEState struct {
	// Instance if the full RBE instance name to use.
	Instance string `json:"instance"`
	// PollToken is base64-encoded HMAC-tagged internalspb.PollState.
	PollToken []byte `json:"poll_token"`
}

// Response is serialized as JSON and sent to the bot.
type Response interface{}

// Server knows how to authenticate bot requests and route them to handlers.
type Server struct {
	router       *router.Router
	middlewares  router.MiddlewareChain
	pollTokenKey atomic.Value // stores secrets.Secret
}

// New constructs new Server.
func New(ctx context.Context, r *router.Router, pollTokenSecret string) (*Server, error) {
	srv := &Server{
		router:      r,
		middlewares: router.MiddlewareChain{
			// TODO(vadimsh): Install authentication middleware.
		},
	}

	// Load the initial value of the key used to HMAC-tag poll tokens.
	key, err := secrets.StoredSecret(ctx, pollTokenSecret)
	if err != nil {
		return nil, err
	}
	srv.pollTokenKey.Store(key)

	// Update the cached value whenever the secret rotates.
	err = secrets.AddRotationHandler(ctx, pollTokenSecret, func(_ context.Context, key secrets.Secret) {
		srv.pollTokenKey.Store(key)
	})
	if err != nil {
		return nil, err
	}

	return srv, nil
}

// InstallHandler installs a bot request handler at the given route.
func (s *Server) InstallHandler(route string, h Handler) {
	s.router.POST(route, s.middlewares, func(c *router.Context) {
		ctx := c.Context
		req := c.Request
		wrt := c.Writer

		// Deserialize JSON request body.
		if ct := req.Header.Get("Content-Type"); strings.ToLower(ct) != "application/json; charset=utf-8" {
			writeErr(ctx, wrt, status.Errorf(codes.InvalidArgument, "bad content type %q", ct))
			return
		}
		var body RequestBody
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeErr(ctx, wrt, status.Errorf(codes.InvalidArgument, "failed to deserialized the request: %s", err))
			return
		}

		// Validate the HMAC tag on the poll token and deserialize it.
		pollState, err := s.validatePollToken(body.RBEState.PollToken)
		if err != nil {
			writeErr(ctx, wrt, status.Errorf(codes.Unauthenticated, "failed to verify poll token: %s", err))
			return
		}

		// Log some information about the request.
		logging.Infof(ctx, "Bot ID: %s", botID(pollState))
		logging.Infof(ctx, "Bot version: %s", body.Version)
		logging.Infof(ctx, "Poll token ID: %s", pollState.Id)
		logging.Infof(ctx, "RBE: %s", pollState.RbeInstance)
		if pollState.DebugInfo != nil {
			logging.Infof(ctx, "Poll token age: %s", clock.Now(ctx).Sub(pollState.DebugInfo.Created.AsTime()))
		}

		// Refuse to use expired tokens.
		if exp := clock.Now(ctx).Sub(pollState.Expiry.AsTime()); exp > 0 {
			writeErr(ctx, wrt, status.Errorf(codes.Unauthenticated, "poll state token expired %s ago", exp))
			return
		}

		// TODO(vadimsh): Verify the bot credentials match what's recorded in the
		// poll token.

		// Apply verified state stored in PollState on top of whatever was reported
		// by the bot. Normally functioning bots should report the same values as
		// stored in the token.
		if body.RBEState.Instance != pollState.RbeInstance {
			logging.Errorf(ctx, "RBE instance mismatch: reported %q, expecting %q",
				body.RBEState.Instance, pollState.RbeInstance,
			)
			body.RBEState.Instance = pollState.RbeInstance
		}
		for _, dim := range pollState.EnforcedDimensions {
			reported := body.Dimensions[dim.Key]
			if !strSliceEq(reported, dim.Values) {
				logging.Errorf(ctx, "Dimension %q mismatch: reported %v, expecting %v",
					dim.Key, reported, dim.Values,
				)
				body.Dimensions[dim.Key] = dim.Values
			}
		}

		// The request is valid, dispatch it to the handler.
		resp, err := h(ctx, &Request{
			Body:      &body,
			PollState: pollState,
		})
		if err != nil {
			writeErr(ctx, wrt, err)
			return
		}

		// Success! Write back the response.
		wrt.Header().Set("Content-Type", "application/json; charset=utf-8")
		var werr error
		if resp == nil {
			_, werr = wrt.Write([]byte("{\"ok\": true}\n"))
		} else {
			werr = json.NewEncoder(wrt).Encode(resp)
		}
		if werr != nil {
			logging.Errorf(ctx, "Error writing the response: %s", werr)
		}
	})
}

// validatePollToken checks the poll token HMAC and deserializes it.
func (s *Server) validatePollToken(tok []byte) (*internalspb.PollState, error) {
	// Deserialize the envelope.
	var envelope internalspb.TaggedMessage
	if err := proto.Unmarshal(tok, &envelope); err != nil {
		return nil, errors.Annotate(err, "failed to deserialize TaggedMessage").Err()
	}
	if envelope.PayloadType != internalspb.TaggedMessage_POLL_STATE {
		return nil, errors.Reason("invalid payload type %v", envelope.PayloadType).Err()
	}

	// Verify the HMAC. It must be produced using any of the secret versions.
	valid := false
	secret := s.pollTokenKey.Load().(secrets.Secret)
	for _, key := range secret.Blobs() {
		// See rbe_pb2.TaggedMessage.
		mac := hmac.New(sha256.New, key)
		_, _ = fmt.Fprintf(mac, "%d\n", envelope.PayloadType)
		_, _ = mac.Write(envelope.Payload)
		expected := mac.Sum(nil)
		if hmac.Equal(expected, envelope.HmacSha256) {
			valid = true
			break
		}
	}
	if !valid {
		return nil, errors.Reason("bad poll token HMAC").Err()
	}

	// The payload can be trusted.
	var payload internalspb.PollState
	if err := proto.Unmarshal(envelope.Payload, &payload); err != nil {
		return nil, errors.Annotate(err, "failed to deserialize PollState").Err()
	}
	return &payload, nil
}

// writeErr logs a gRPC error and writes it to the HTTP response.
//
// The error can be wrapped.
func writeErr(ctx context.Context, w http.ResponseWriter, err error) {
	err = grpcutil.GRPCifyAndLogErr(ctx, err)
	httpCode := grpcutil.CodeStatus(status.Code(err))
	logging.Errorf(ctx, "HTTP %d: %s", httpCode, err)
	http.Error(w, err.Error(), httpCode)
}

// botID extracts the bot ID from PollState, for logs.
func botID(s *internalspb.PollState) string {
	for _, dim := range s.EnforcedDimensions {
		if dim.Key == "id" {
			if len(dim.Values) > 0 {
				return dim.Values[0]
			}
			return "<unknown>"
		}
	}
	return "<unknown>"
}

// strSliceEq is true if two string slices are equal.
func strSliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
