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
// It checks PollState/BotSession tokens and bot credentials.
package botsrv

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/prototext"
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
	"go.chromium.org/luci/tokenserver/auth/machine"

	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/hmactoken"
)

// RequestBody should be implemented by a JSON-serializable struct representing
// format of some particular request.
type RequestBody interface {
	ExtractPollToken() []byte               // the poll token, if present
	ExtractSessionToken() []byte            // the session token, if present
	ExtractDimensions() map[string][]string // dimensions reported by the bot, if present
	ExtractDebugRequest() any               // serialized as JSON and logged on errors
}

// Request is extracted from an authenticated request from a bot.
type Request struct {
	BotID               string                 // validated bot ID
	SessionID           string                 // validated RBE bot session ID, if present
	SessionTokenExpired bool                   // true if the request has expired session token
	PollState           *internalspb.PollState // validated poll state
	Dimensions          map[string][]string    // validated dimensions
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
	router      *router.Router
	middlewares router.MiddlewareChain
	hmacSecret  *hmactoken.Secret
}

// New constructs new Server.
func New(ctx context.Context, r *router.Router, projectID string, hmacSecret *hmactoken.Secret) *Server {
	gaeAppDomain := fmt.Sprintf("%s.appspot.com", projectID)
	return &Server{
		router: r,
		middlewares: router.MiddlewareChain{
			// All supported bot authentication schemes. The first matching one wins.
			auth.Authenticate(
				// This checks "X-Luci-Gce-Vm-Token" header if present. The token
				// audience should be `[https://][<prefix>-dot-]app.appspot.com`.
				&openid.GoogleComputeAuthMethod{
					Header: "X-Luci-Gce-Vm-Token",
					AudienceCheck: func(_ context.Context, _ auth.RequestMetadata, aud string) (bool, error) {
						aud = strings.TrimPrefix(aud, "https://")
						return aud == gaeAppDomain || strings.HasSuffix(aud, "-dot-"+gaeAppDomain), nil
					},
				},
				// This checks "X-Luci-Machine-Token" header if present.
				&machine.MachineTokenAuthMethod{},
				// This checks "Authorization" header if present.
				&auth.GoogleOAuth2Method{
					Scopes: []string{"https://www.googleapis.com/auth/userinfo.email"},
				},
			),
		},
		hmacSecret: hmacSecret,
	}
}

// RequestBodyConstraint is needed to make Go generics type checker happy.
type RequestBodyConstraint[B any] interface {
	RequestBody
	*B
}

// InstallHandler installs a bot request handler at the given route.
func InstallHandler[B any, RB RequestBodyConstraint[B]](s *Server, route string, h Handler[B]) {
	s.router.POST(route, s.middlewares, func(c *router.Context) {
		ctx := c.Request.Context()
		req := c.Request
		wrt := c.Writer

		// Deserialized request body.
		var body *B

		// Deserialized and validated tokens in the request.
		var pollTokenState *internalspb.PollState
		var sessionState *internalspb.BotSession

		// This is either pollTokenState or the poll state inside sessionState,
		// depending on which token is non-expired. Populated below.
		var pollState *internalspb.PollState

		// writeErr logs a gRPC error and writes it to the HTTP response.
		writeErr := func(err error) {
			// Log request details to help in debugging errors.
			logging.Infof(ctx, "Bot IP: %s", auth.GetState(ctx).PeerIP())
			logging.Infof(ctx, "Authenticated: %s", auth.GetState(ctx).PeerIdentity())
			if pollState != nil {
				logging.Infof(ctx, "Bot ID: %s", extractBotID(pollState))
				logging.Infof(ctx, "Poll token ID: %s", pollState.Id)
				logging.Infof(ctx, "RBE: %s", pollState.RbeInstance)
				if pollState.DebugInfo != nil {
					logging.Infof(ctx, "Poll token age: %s", clock.Now(ctx).Sub(pollState.DebugInfo.Created.AsTime()))
				}
			}
			if sessionState != nil {
				logging.Infof(ctx, "Session ID: %s", sessionState.RbeBotSessionId)
			}
			if body != nil {
				blob, _ := json.MarshalIndent(RB(body).ExtractDebugRequest(), "", "  ")
				logging.Infof(ctx, "Request body:\n%s", blob)
			}

			// Log the actual error.
			err = grpcutil.GRPCifyAndLogErr(ctx, err)
			statusCode := status.Code(err)
			httpCode := grpcutil.CodeStatus(statusCode)
			if statusCode == codes.Unavailable {
				// UNAVAILABLE seems to happen a lot, but in bursts (probably when the
				// RBE scheduler restarts). Log it at the warning severity to make other
				// errors more noticeable.
				logging.Warningf(ctx, "HTTP %d: %s", httpCode, err)
			} else {
				logging.Errorf(ctx, "HTTP %d: %s", httpCode, err)
			}

			http.Error(wrt, err.Error(), httpCode)
		}

		// Deserialize JSON request body.
		if ct := req.Header.Get("Content-Type"); strings.ToLower(ct) != "application/json; charset=utf-8" {
			writeErr(status.Errorf(codes.InvalidArgument, "bad content type %q", ct))
			return
		}
		raw, err := io.ReadAll(req.Body)
		if err != nil {
			writeErr(status.Errorf(codes.Internal, "error reading request body: %s", err))
			return
		}
		body = new(B)
		if err := json.Unmarshal(raw, body); err != nil {
			logging.Warningf(ctx, "Unrecognized request:\n%s", raw)
			writeErr(status.Errorf(codes.InvalidArgument, "failed to deserialized the request: %s", err))
			return
		}

		// To authenticate the bot we need either a non-expired poll token, a
		// non-expired session token or both (in which case the poll token is
		// preferred, since it should be more recently produced in this case). If we
		// have a poll token, we validate it to directly get PollState. If we have
		// a session token, we validate it and grab PollState from within it. This
		// PollState is then used to check bot credentials.
		//
		// This scheme is necessary because poll tokens can be produced only by
		// Python Swarming server when bot calls "/bot/poll" endpoint. When the bot
		// is running a task, it isn't polling Python Swarming server and its poll
		// token expires. For that reason when running a task (or making other
		// post-task calls that happen before the next poll), we use the session
		// token instead, which has the most recently validated PollState stored in
		// it in a "frozen" state.
		//
		// When the bot is polling for tasks, it sends both poll token and session
		// token to us, which allows us to put up-to-date PollState into the
		// session token. This happens in UpdateBotSession handler.

		// If have a poll token, validate and deserialize it.
		if pollToken := RB(body).ExtractPollToken(); len(pollToken) != 0 {
			pollTokenState = &internalspb.PollState{}
			if err := s.hmacSecret.ValidateToken(pollToken, pollTokenState); err != nil {
				writeErr(status.Errorf(codes.Unauthenticated, "failed to verify poll token: %s", err))
				return
			}
			if exp := clock.Now(ctx).Sub(pollTokenState.Expiry.AsTime()); exp > 0 {
				logging.Warningf(ctx, "Ignoring poll token (expired %s ago):\n%s", exp, prettyProto(pollTokenState))
				pollTokenState = nil
			}
		}
		// If have a session token, validate and deserialize it as well.
		sessionTokenExpired := false
		if sessionToken := RB(body).ExtractSessionToken(); len(sessionToken) != 0 {
			sessionState = &internalspb.BotSession{}
			if err := s.hmacSecret.ValidateToken(sessionToken, sessionState); err != nil {
				writeErr(status.Errorf(codes.Unauthenticated, "failed to verify session token: %s", err))
				return
			}
			if exp := clock.Now(ctx).Sub(sessionState.Expiry.AsTime()); exp > 0 {
				logging.Warningf(ctx, "Ignoring session token (expired %s ago):\n%s", exp, prettyProto(sessionState))
				sessionState = nil
				sessionTokenExpired = true
			}
		}

		// Need at least one valid and fresh token.
		if pollTokenState == nil && sessionState == nil {
			writeErr(status.Errorf(codes.Unauthenticated, "no valid poll or state token"))
			return
		}

		// Prefer the state from the poll token. It is fresher. Fallback to the
		// state stored in the session token if there's no poll token or it has
		// expired.
		pollState = pollTokenState
		if pollState == nil {
			pollState = sessionState.GetPollState()
			if pollState == nil {
				writeErr(status.Errorf(codes.Unauthenticated, "no poll state available"))
				return
			}
		}

		// Extract bot ID from the validated PollToken.
		botID := extractBotID(pollState)
		if botID == "" {
			writeErr(status.Errorf(codes.InvalidArgument, "no bot ID"))
			return
		}
		// Session ID must be present if there's a session token.
		if sessionState != nil && sessionState.RbeBotSessionId == "" {
			writeErr(status.Errorf(codes.InvalidArgument, "no session ID"))
			return
		}

		// Verify bot credentials match what's recorded in the validated poll state.
		if err := checkCredentials(ctx, pollState); err != nil {
			if transient.Tag.In(err) {
				writeErr(status.Errorf(codes.Internal, "transient error checking bot credentials: %s", err))
			} else {
				writeErr(status.Errorf(codes.Unauthenticated, "bad bot credentials: %s", err))
			}
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

		// There must be `pool` dimension with at least one value (perhaps more).
		if len(dims["pool"]) == 0 {
			writeErr(status.Errorf(codes.InvalidArgument, "no pool dimension"))
			return
		}

		// The request is valid, dispatch it to the handler.
		resp, err := h(ctx, body, &Request{
			BotID:               botID,
			SessionID:           sessionState.GetRbeBotSessionId(),
			SessionTokenExpired: sessionTokenExpired,
			PollState:           pollState,
			Dimensions:          dims,
		})
		if err != nil {
			writeErr(err)
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

// prettyProto formats a proto message for logs.
func prettyProto(msg proto.Message) string {
	blob, err := prototext.MarshalOptions{
		Multiline: true,
		Indent:    "  ",
	}.Marshal(msg)
	if err != nil {
		return fmt.Sprintf("<error: %s>", err)
	}
	return string(blob)
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
		switch yes, err := auth.IsAllowedIP(ctx, pollState.IpAllowlist); {
		case err != nil:
			return errors.Annotate(err, "IP allowlist check failed").Tag(transient.Tag).Err()
		case !yes:
			return errors.Reason("bot IP %s is not in the allowlist", auth.GetState(ctx).PeerIP()).Err()
		}
	}

	return nil
}

// extractBotID extracts the bot ID from PollState.
//
// Returns "" if it is not present.
func extractBotID(s *internalspb.PollState) string {
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
