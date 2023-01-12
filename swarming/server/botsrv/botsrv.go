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

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/tokenserver/auth/machine"

	internalspb "go.chromium.org/luci/swarming/proto/internals"
)

// RequestBody should be implemented by a JSON-serializable struct representing
// format of some particular request.
type RequestBody interface {
	ExtractPollToken() []byte               // the poll token, if present
	ExtractDimensions() map[string][]string // dimensions reported by the bot, if present
}

// Request is extracted from an authenticated request from a bot.
type Request struct {
	BotID      string                 // validated bot ID
	PollState  *internalspb.PollState // validated poll state
	Dimensions map[string][]string    // validated dimensions
}

// Response is serialized as JSON and sent to the bot.
type Response any

// Handler handles an authenticated request from a bot.
//
// It takes a raw deserialized request body and all authenticated data extracted
// from it.
//
// It returns a response that will be serialized and sent to the bot as JSON or
// a gRPC error code that will be converted into an HTTP error.
type Handler[B any] func(ctx context.Context, body *B, req *Request) (Response, error)

// Server knows how to authenticate bot requests and route them to handlers.
type Server struct {
	router        *router.Router
	middlewares   router.MiddlewareChain
	hmacSecretKey atomic.Value // stores secrets.Secret
}

// New constructs new Server.
func New(ctx context.Context, r *router.Router, hmacSecret string) (*Server, error) {
	srv := &Server{
		router: r,
		middlewares: router.MiddlewareChain{
			// All supported bot authentication schemes. The first matching one wins.
			auth.Authenticate(
				// This checks "X-Luci-Gce-Vm-Token" header if present.
				&openid.GoogleComputeAuthMethod{
					Header:        "X-Luci-Gce-Vm-Token",
					AudienceCheck: openid.AudienceMatchesHost,
				},
				// This checks "X-Luci-Machine-Token" header if present.
				&machine.MachineTokenAuthMethod{},
				// This checks "Authorization" header if present.
				&auth.GoogleOAuth2Method{
					Scopes: []string{"https://www.googleapis.com/auth/userinfo.email"},
				},
			),
		},
	}

	// Load the initial value of the key used to HMAC-tag poll tokens.
	key, err := secrets.StoredSecret(ctx, hmacSecret)
	if err != nil {
		return nil, err
	}
	srv.hmacSecretKey.Store(key)

	// Update the cached value whenever the secret rotates.
	err = secrets.AddRotationHandler(ctx, hmacSecret, func(_ context.Context, key secrets.Secret) {
		srv.hmacSecretKey.Store(key)
	})
	if err != nil {
		return nil, err
	}

	return srv, nil
}

// RequestBodyConstraint is needed to make Go generics type checker happy.
type RequestBodyConstraint[B any] interface {
	RequestBody
	*B
}

// InstallHandler installs a bot request handler at the given route.
func InstallHandler[B any, RB RequestBodyConstraint[B]](s *Server, route string, h Handler[B]) {
	s.router.POST(route, s.middlewares, func(c *router.Context) {
		ctx := c.Context
		req := c.Request
		wrt := c.Writer

		// Deserialize JSON request body.
		if ct := req.Header.Get("Content-Type"); strings.ToLower(ct) != "application/json; charset=utf-8" {
			writeErr(ctx, wrt, status.Errorf(codes.InvalidArgument, "bad content type %q", ct))
			return
		}
		body := new(B)
		if err := json.NewDecoder(req.Body).Decode(body); err != nil {
			writeErr(ctx, wrt, status.Errorf(codes.InvalidArgument, "failed to deserialized the request: %s", err))
			return
		}

		// Validate the HMAC tag on the poll token and deserialize it.
		pollState := &internalspb.PollState{}
		if err := s.validateToken(RB(body).ExtractPollToken(), pollState); err != nil {
			writeErr(ctx, wrt, status.Errorf(codes.Unauthenticated, "failed to verify poll token: %s", err))
			return
		}

		// Extract bot ID from the validated PollToken.
		botID := botID(pollState)

		// Log some information about the request.
		logging.Infof(ctx, "Bot ID: %s", botID)
		logging.Infof(ctx, "Bot IP: %s", auth.GetState(ctx).PeerIP())
		logging.Infof(ctx, "Authenticated: %s", auth.GetState(ctx).PeerIdentity())
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

		// Verify bot credentials match what's recorded in the poll token.
		if err := checkCredentials(ctx, pollState); err != nil {
			if transient.Tag.In(err) {
				writeErr(ctx, wrt, status.Errorf(codes.Internal, "transient error checking bot credentials: %s", err))
			} else {
				writeErr(ctx, wrt, status.Errorf(codes.Unauthenticated, "bad bot credentials: %s", err))
			}
			return
		}

		// Bot ID must be present.
		if botID == "" {
			writeErr(ctx, wrt, status.Errorf(codes.InvalidArgument, "no bot ID"))
			return
		}

		// Apply verified state stored in PollState on top of whatever was reported
		// by the bot. Normally functioning bots should report the same values as
		// stored in the token.
		dims := RB(body).ExtractDimensions()
		for _, dim := range pollState.EnforcedDimensions {
			reported := dims[dim.Key]
			if !strSliceEq(reported, dim.Values) {
				logging.Errorf(ctx, "Dimension %q mismatch: reported %v, expecting %v",
					dim.Key, reported, dim.Values,
				)
				dims[dim.Key] = dim.Values
			}
		}

		// The request is valid, dispatch it to the handler.
		resp, err := h(ctx, body, &Request{
			BotID:      botID,
			PollState:  pollState,
			Dimensions: dims,
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

// validateToken deserializes a TaggedMessage, checks the HMAC and deserializes
// the payload into `msg`.
func (s *Server) validateToken(tok []byte, msg proto.Message) error {
	// Deserialize the envelope.
	var envelope internalspb.TaggedMessage
	if err := proto.Unmarshal(tok, &envelope); err != nil {
		return errors.Annotate(err, "failed to deserialize TaggedMessage").Err()
	}
	if expected := taggedMessagePayload(msg); envelope.PayloadType != expected {
		return errors.Reason("invalid payload type %v, expecting %v", envelope.PayloadType, expected).Err()
	}

	// Verify the HMAC. It must be produced using any of the secret versions.
	valid := false
	secret := s.hmacSecretKey.Load().(secrets.Secret)
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
		return errors.Reason("bad token HMAC").Err()
	}

	// The payload can be trusted.
	if err := proto.Unmarshal(envelope.Payload, msg); err != nil {
		return errors.Annotate(err, "failed to deserialize token payload").Err()
	}
	return nil
}

// generateToken wraps `msg` into a serialized TaggedMessage.
//
// The produced token can be validated and deserialized with validateToken.
func (s *Server) generateToken(msg proto.Message) ([]byte, error) {
	payload, err := proto.Marshal(msg)
	if err != nil {
		return nil, errors.Annotate(err, "failed to serialize the token payload").Err()
	}

	// The future token, but without HMAC yet.
	envelope := internalspb.TaggedMessage{
		PayloadType: taggedMessagePayload(msg),
		Payload:     payload,
	}

	// See rbe_pb2.TaggedMessage.
	secret := s.hmacSecretKey.Load().(secrets.Secret).Active
	mac := hmac.New(sha256.New, secret)
	_, _ = fmt.Fprintf(mac, "%d\n", envelope.PayloadType)
	_, _ = mac.Write(envelope.Payload)
	envelope.HmacSha256 = mac.Sum(nil)

	token, err := proto.Marshal(&envelope)
	if err != nil {
		return nil, errors.Annotate(err, "failed to serialize the token").Err()
	}
	return token, nil
}

// taggedMessagePayload examines the type of msg and returns the corresponding
// enum variant.
//
// Panics if it is a completely unexpected message.
func taggedMessagePayload(msg proto.Message) internalspb.TaggedMessage_PayloadType {
	switch msg.(type) {
	case *internalspb.PollState:
		return internalspb.TaggedMessage_POLL_STATE
	case *internalspb.BotSession:
		return internalspb.TaggedMessage_BOT_SESSION
	default:
		panic(fmt.Sprintf("unexpected message type %T", msg))
	}
}

// checkCredentials checks the bot credentials in the context match what is
// required by the PollState.
//
// It ensures the Go portion of the Swarming server authenticates the bot in
// the exact same way the Python portion did (since the Python portion produced
// the PollState after it authenticated the bot).
func checkCredentials(ctx context.Context, pollState *internalspb.PollState) error {
	switch m := pollState.AuthMethod.(type) {
	case *internalspb.PollState_GceAuth:
		gceInfo := openid.GetGoogleComputeTokenInfo(ctx)
		if gceInfo == nil {
			return errors.Reason("expecting GCE VM token auth").Err()
		}
		if gceInfo.Project != m.GceAuth.GceProject || gceInfo.Instance != m.GceAuth.GceInstance {
			logging.Errorf(ctx, "Bad GCE VM auth: want %s@%s, got %s@%s",
				m.GceAuth.GceInstance, m.GceAuth.GceProject,
				gceInfo.Instance, gceInfo.Project,
			)
			return errors.Reason("wrong GCE VM token: %s@%s", gceInfo.Instance, gceInfo.Project).Err()
		}

	case *internalspb.PollState_ServiceAccountAuth_:
		peerID := auth.GetState(ctx).PeerIdentity()
		if peerID.Kind() != identity.User {
			return errors.Reason("expecting service account credentials").Err()
		}
		if peerID.Email() != m.ServiceAccountAuth.ServiceAccount {
			logging.Errorf(ctx, "Bad service account auth: want %s, got %s",
				m.ServiceAccountAuth.ServiceAccount,
				peerID.Email(),
			)
			return errors.Reason("wrong service account: %s", peerID.Email()).Err()
		}

	case *internalspb.PollState_LuciMachineTokenAuth:
		tokInfo := machine.GetMachineTokenInfo(ctx)
		if tokInfo == nil {
			return errors.Reason("expecting LUCI machine token auth").Err()
		}
		if tokInfo.FQDN != m.LuciMachineTokenAuth.MachineFqdn {
			logging.Errorf(ctx, "Bad LUCI machine token FQDN: want %s, got %s",
				m.LuciMachineTokenAuth.MachineFqdn,
				tokInfo.FQDN,
			)
			return errors.Reason("wrong FQDN in the LUCI machine token: %s", tokInfo.FQDN).Err()
		}

	case *internalspb.PollState_IpAllowlistAuth:
		// The actual check is below. Here we just verify the PollState token is
		// consistent.
		if pollState.IpAllowlist == "" {
			return errors.Reason("bad poll token: using IP allowlist auth without an IP allowlist").Err()
		}

	default:
		return errors.Reason("unrecognized auth method in the poll token: %v", pollState.AuthMethod).Err()
	}

	// Verify the bot is in the required IP allowlist (if any).
	if pollState.IpAllowlist != "" {
		switch yes, err := auth.IsInWhitelist(ctx, pollState.IpAllowlist); {
		case err != nil:
			return errors.Annotate(err, "IP allowlist check failed").Tag(transient.Tag).Err()
		case !yes:
			return errors.Reason("bot IP %s is not in the allowlist", auth.GetState(ctx).PeerIP()).Err()
		}
	}

	return nil
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

// botID extracts the bot ID from PollState.
//
// Returns "" if it is not present.
func botID(s *internalspb.PollState) string {
	for _, dim := range s.EnforcedDimensions {
		if dim.Key == "id" {
			if len(dim.Values) > 0 {
				return dim.Values[0]
			}
			return ""
		}
	}
	return ""
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
