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

// Package botsrv knows how to handle calls from Swarming bots.
package botsrv

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/tokenserver/auth/machine"

	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/botsession"
	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/hmactoken"
	"go.chromium.org/luci/swarming/server/pyproxy"
)

// RequestBody should be implemented by a JSON-serializable struct representing
// format of some particular request.
type RequestBody interface {
	ExtractSession() []byte   // the token with bot Session proto
	ExtractDebugRequest() any // serialized as JSON and logged on errors
}

// Request is extracted from an authenticated request from a bot and the bot
// state in the datastore.
type Request struct {
	Session       *internalspb.Session // the bot session from the session token
	Dimensions    BotDimensions        // bot's "k:v" dimensions as stored in the datastore
	CurrentTaskID string               // the TaskRunResult key of the current task as store in the datastore
}

// BotDimensions is a sorted list of bot's "k:v" dimensions.
type BotDimensions []string

// DimensionValues extracts dimension values from a list of flat dimensions.
func (dims BotDimensions) DimensionValues(key string) []string {
	var out []string
	for _, kv := range dims {
		if k, v, ok := strings.Cut(kv, ":"); ok && k == key {
			out = append(out, v)
		}
	}
	return out
}

// ToMap converts dimensions to a map form used in some APIs.
func (dims BotDimensions) ToMap() map[string][]string {
	m := map[string][]string{}
	for _, kv := range dims {
		if k, v, ok := strings.Cut(kv, ":"); ok {
			m[k] = append(m[k], v)
		}
	}
	return m
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

// KnownBotInfo is information about a bot registered in the datastore.
type KnownBotInfo struct {
	// SessionID is the current bot session ID of this bot.
	SessionID string
	// Dimensions is "k:v" dimensions registered by this bot in the last poll.
	Dimensions BotDimensions
	// CurrentTaskID is the TaskRunResult key of the current task on the bot.
	CurrentTaskID string
}

// KnownBotProvider knows how to return information about existing bots.
//
// Returns nil and no error if the bot is not registered in the datastore. All
// other errors can be considered transient.
type KnownBotProvider func(ctx context.Context, botID string) (*KnownBotInfo, error)

// Server knows how to authenticate bot requests and route them to handlers.
type Server struct {
	router      *router.Router
	middlewares router.MiddlewareChain
	hmacSecret  *hmactoken.Secret
	cfg         *cfg.Provider
	knownBots   KnownBotProvider
}

// New constructs new Server.
func New(ctx context.Context, cfg *cfg.Provider, r *router.Router, prx *pyproxy.Proxy, bots KnownBotProvider, projectID string, hmacSecret *hmactoken.Secret) *Server {
	gaeAppDomain := fmt.Sprintf("%s.appspot.com", projectID)

	// Redirect to Python for eligible requests before hitting any other
	// middlewares.
	var middlewares router.MiddlewareChain
	if prx != nil {
		middlewares = append(middlewares, pythonProxyMiddleware(prx))
	}

	return &Server{
		router: r,
		middlewares: append(middlewares,
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
		),
		hmacSecret: hmacSecret,
		cfg:        cfg,
		knownBots:  bots,
	}
}

// pythonProxyMiddleware is a middleware that routes a portion of requests to
// the python server.
func pythonProxyMiddleware(prx *pyproxy.Proxy) router.Middleware {
	return func(c *router.Context, next router.Handler) {
		// Bot API routes "/something/:Parameter" and "/something" are the same.
		// Strip "/:Parameter" part when looking up the routing percent.
		routeName := c.HandlerPath
		chunks := strings.Split(c.HandlerPath, "/")
		if strings.HasPrefix(chunks[len(chunks)-1], ":") {
			routeName = strings.Join(chunks[:len(chunks)-1], "/")
		}
		if strings.HasPrefix(routeName, "/swarming/api/v1/bot/rbe/") {
			// Never proxy to Python requests that are implemented only in Go.
			next(c)
		} else if !prx.DefaultOverride(routeName, c.Writer, c.Request) {
			// DefaultOverride returned false => need to handle the request in Go.
			next(c)
		}
	}
}

// RequestBodyConstraint is needed to make Go generics type checker happy.
type RequestBodyConstraint[B any] interface {
	RequestBody
	*B
}

// GET installs a GET request handler.
//
// It authenticates the bot or user credentials (if any), but doesn't itself
// check bots.cfg authorization rules. Call AuthorizeBot to do that.
//
// It additionally applies traffic routing rules to send a portion of requests
// to the Python server.
func GET(s *Server, route string, handler router.Handler) {
	s.router.GET(route, s.middlewares, handler)
}

// JSON installs a bot API request handler at the given route.
//
// This is a POST handler that receives JSON-serialized B and replies with
// some JSON-serialized response.
//
// It performs bot authentication and authorization based on the bot session
// token and the bot state in the datastore.
func JSON[B any, RB RequestBodyConstraint[B]](s *Server, route string, h Handler[B]) {
	s.router.POST(route, s.middlewares, func(c *router.Context) {
		ctx := c.Request.Context()
		req := c.Request
		wrt := c.Writer

		// Deserialize JSON request body.
		body := new(B)
		if err := readRequest(req, body); err != nil {
			writeErr(err, req, wrt, nil, nil)
			return
		}

		// A valid non-expired session token is required to authenticate the bot.
		sessionTok := RB(body).ExtractSession()
		if len(sessionTok) == 0 {
			writeErr(status.Errorf(codes.Unauthenticated, "no session token"), req, wrt, RB(body), nil)
			return
		}
		session, err := botsession.CheckSessionToken(sessionTok, s.hmacSecret, clock.Now(ctx))
		if err != nil {
			writeErr(err, req, wrt, RB(body), session)
			return
		}

		// Verify the bot credentials match what's recorded in the session token.
		// Since the session token is initially created by the server after it
		// authorizes the bot, this check verifies the bot is known per the config
		// when the token was initially created.
		//
		// TODO: Add a check for session.bot_config.expiry to limit how long a bot
		// can reuse the "captured" config. This depends on actually populating this
		// expiry correctly before launching long-running tasks.
		if err := AuthorizeBot(ctx, session.BotId, session.BotConfig.BotAuth); err != nil {
			if transient.Tag.In(err) {
				writeErr(status.Errorf(codes.Internal, "transient error checking bot credentials: %s", err), req, wrt, RB(body), session)
			} else {
				writeErr(status.Errorf(codes.Unauthenticated, "bad bot credentials: %s", err), req, wrt, RB(body), session)
			}
			return
		}

		// Verify this bot is known in the datastore and fetch its registered
		// dimensions. Bots are registered in /handshake or /bot/poll.
		knownBot, err := s.knownBots(ctx, session.BotId)
		switch {
		case err != nil:
			writeErr(status.Errorf(codes.Internal, "error fetching bot info: %s", err), req, wrt, RB(body), session)
			return
		case knownBot == nil:
			writeErr(status.Errorf(codes.PermissionDenied, "%q is not a registered bot", session.BotId), req, wrt, RB(body), session)
			return
		}

		// TODO: Deal with concurrent sessions by "gracefully" closing an older one.
		// Refusing an unexpected session is also an only way to quickly revoke
		// access to a bot (before the session token expires).
		if knownBot.SessionID != session.SessionId {
			logging.Warningf(ctx, "Wrong session ID: expect %q, got %q", knownBot.SessionID, session.SessionId)
		}

		// Verify that required dimensions are present. This is enforced when the
		// bot is registered. This check is here to avoid panics up the stack if
		// something in the datastore is wrong for some reason.
		if !slices.Equal(knownBot.Dimensions.DimensionValues("id"), []string{session.BotId}) {
			writeErr(status.Errorf(codes.Internal, `wrong stored "id" dimension`), req, wrt, RB(body), session)
			return
		}
		if len(knownBot.Dimensions.DimensionValues("pool")) == 0 {
			writeErr(status.Errorf(codes.Internal, `no stored "pool" dimension`), req, wrt, RB(body), session)
			return
		}

		// The request is valid, dispatch it to the handler.
		resp, err := h(ctx, body, &Request{
			Session:       session,
			Dimensions:    knownBot.Dimensions,
			CurrentTaskID: knownBot.CurrentTaskID,
		})
		if err != nil {
			writeErr(err, req, wrt, RB(body), session)
		} else {
			writeResponse(req, wrt, resp)
		}
	})
}

// NoSessionJSON is like JSON handler, except it doesn't check the session.
//
// It authenticates the bot or user credentials (if any), but doesn't itself
// check bots.cfg authorization rules or the session token.
//
// This is needed for the handshake and bot poll handlers since they need to
// handle missing or expired session tokens themselves. These handlers do
// necessary authorization checks inside.
func NoSessionJSON[B any, RB RequestBodyConstraint[B]](s *Server, route string, h Handler[B]) {
	s.router.POST(route, s.middlewares, func(c *router.Context) {
		ctx := c.Request.Context()
		req := c.Request
		wrt := c.Writer

		body := new(B)
		if err := readRequest(req, body); err != nil {
			writeErr(err, req, wrt, nil, nil)
			return
		}

		resp, err := h(ctx, body, &Request{})
		if err != nil {
			writeErr(err, req, wrt, RB(body), nil)
		} else {
			writeResponse(req, wrt, resp)
		}
	})
}

// readRequest reads JSON request body.
//
// Returns gRPC status errors.
func readRequest(req *http.Request, body any) error {
	if ct := req.Header.Get("Content-Type"); strings.ToLower(ct) != "application/json; charset=utf-8" {
		return status.Errorf(codes.InvalidArgument, "bad content type %q", ct)
	}
	raw, err := io.ReadAll(req.Body)
	if err != nil {
		return status.Errorf(codes.Internal, "error reading request body: %s", err)
	}
	if err := json.Unmarshal(raw, body); err != nil {
		logging.Warningf(req.Context(), "Unrecognized request:\n%s", raw)
		return status.Errorf(codes.InvalidArgument, "failed to deserialized the request: %s", err)
	}
	return nil
}

// writeResponse writes a successful response.
func writeResponse(req *http.Request, rw http.ResponseWriter, resp Response) {
	rw.Header().Set("Content-Type", "application/json; charset=utf-8")
	var werr error
	if resp == nil {
		_, werr = rw.Write([]byte("{\"ok\": true}\n"))
	} else {
		werr = json.NewEncoder(rw).Encode(resp)
	}
	if werr != nil {
		logging.Errorf(req.Context(), "Error writing the response: %s", werr)
	}
}

// writeErr logs an error (with request details) and sends it as the response.
//
// Expects gRPC error as input, using its code to derive HTTP status code.
func writeErr(err error, req *http.Request, rw http.ResponseWriter, body RequestBody, session *internalspb.Session) {
	ctx := req.Context()

	// Log request details to help in debugging errors.
	logging.Infof(ctx, "Bot IP: %s", auth.GetState(ctx).PeerIP())
	logging.Infof(ctx, "Authenticated: %s", auth.GetState(ctx).PeerIdentity())
	if session != nil {
		botsession.LogSession(ctx, session)
	}
	if body != nil {
		blob, _ := json.MarshalIndent(body.ExtractDebugRequest(), "", "  ")
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

	http.Error(rw, err.Error(), httpCode)
}
