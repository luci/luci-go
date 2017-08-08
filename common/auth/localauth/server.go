// Copyright 2017 The LUCI Authors.
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

package localauth

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"regexp"
	"sort"
	"sync"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/auth/localauth/rpcs"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/runtime/paniccatcher"
	"go.chromium.org/luci/lucictx"
)

// TokenGenerator produces access tokens.
//
// It is called to return an access token for given combination of scopes (given
// as a sorted list of strings without duplicates).
//
// It is called for each request to the local auth server. It may be called
// concurrently from multiple goroutines and must implement its own caching and
// synchronization if necessary.
//
// It is expected that the returned token lives for at least given 'lifetime'
// duration (which is typically on order of minutes), but it may live longer.
// Clients may cache the returned token for the duration of its lifetime.
//
// May return transient errors (in transient.Tag.In(err) returning true
// sense). Such errors result in HTTP 500 responses. This is appropriate for
// non-fatal errors. Clients may immediately retry requests on such errors.
//
// Any non-transient error is considered fatal and results in an RPC-level
// error response ({"error": ...}). Clients must treat such responses as fatal
// and don't retry requests.
//
// If the error implements ErrorWithCode interface, the error code returned to
// clients will be grabbed from the error object, otherwise the error code is
// set to -1.
type TokenGenerator func(ctx context.Context, scopes []string, lifetime time.Duration) (*oauth2.Token, error)

// ErrorWithCode is a fatal error that also has a numeric code.
//
// May be returned by TokenGenerator to trigger a response with some specific
// error code.
type ErrorWithCode interface {
	error

	// Code returns a code to put into RPC response alongside the error message.
	Code() int
}

// Server runs a local RPC server that hands out access tokens.
//
// Processes that need a token can discover location of this server by looking
// at "local_auth" section of LUCI_CONTEXT.
type Server struct {
	// TokenGenerators produce access tokens for given account IDs.
	TokenGenerators map[string]TokenGenerator

	// DefaultAccountID is account ID subprocesses should pick by default.
	//
	// It is put into "local_auth" section of LUCI_CONTEXT. If empty string,
	// subprocesses won't attempt to use any account by default (they still can
	// pick some non-default account though).
	DefaultAccountID string

	// Port is a local TCP port to bind to or 0 to allow the OS to pick one.
	Port int

	l        sync.Mutex
	secret   []byte             // the clients are expected to send this secret
	listener net.Listener       // to know what to stop in Close, nil after Close
	wg       sync.WaitGroup     // +1 for each request being processed now
	ctx      context.Context    // derived from ctx in Initialize
	cancel   context.CancelFunc // cancels 'ctx'

	testingServeHook func() // called right before serving
}

// Initialize binds the server to a local port and prepares it for serving.
//
// The provided context is used as base context for request handlers and for
// logging.
//
// Returns a copy of lucictx.LocalAuth structure that specifies how to contact
// the server. It should be put into "local_auth" section of LUCI_CONTEXT where
// clients can discover it.
func (s *Server) Initialize(ctx context.Context) (*lucictx.LocalAuth, error) {
	s.l.Lock()
	defer s.l.Unlock()

	if s.ctx != nil {
		return nil, fmt.Errorf("already initialized")
	}

	secret := make([]byte, 48)
	if _, err := cryptorand.Read(ctx, secret); err != nil {
		return nil, err
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.Port))
	if err != nil {
		return nil, err
	}

	s.ctx, s.cancel = context.WithCancel(ctx)
	s.listener = ln
	s.secret = secret

	// Build a sorted list of LocalAuthAccount to put into the context.
	ids := make([]string, 0, len(s.TokenGenerators))
	for id := range s.TokenGenerators {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	accounts := make([]lucictx.LocalAuthAccount, len(ids))
	for i, id := range ids {
		accounts[i] = lucictx.LocalAuthAccount{ID: id}
	}

	return &lucictx.LocalAuth{
		RPCPort:          uint32(ln.Addr().(*net.TCPAddr).Port),
		Secret:           secret,
		Accounts:         accounts,
		DefaultAccountID: s.DefaultAccountID,
	}, nil
}

// Serve runs a serving loop.
//
// It unblocks once Close is called and all pending requests are served.
//
// Returns nil if serving was stopped by Close or non-nil if it failed for some
// other reason.
func (s *Server) Serve() (err error) {
	s.l.Lock()
	switch {
	case s.ctx == nil:
		err = fmt.Errorf("not initialized")
	case s.listener == nil:
		err = fmt.Errorf("already closed")
	}
	if err != nil {
		s.l.Unlock()
		return
	}
	listener := s.listener // accessed outside the lock
	srv := http.Server{
		Handler: &protocolHandler{
			ctx:    s.ctx,
			wg:     &s.wg,
			secret: s.secret,
			tokens: s.TokenGenerators,
		},
	}
	s.l.Unlock()

	// Notify unit tests that we have initialized.
	if s.testingServeHook != nil {
		s.testingServeHook()
	}

	err = srv.Serve(listener) // blocks until Close() is called
	s.wg.Wait()               // waits for all pending requests to finish

	// If it was a planned shutdown with Close(), ignore the error. It says that
	// the listening socket was closed.
	s.l.Lock()
	if s.listener == nil {
		err = nil
	}
	s.l.Unlock()

	return
}

// Close closes the listening socket and notifies pending requests to abort.
//
// Safe to call multiple times.
func (s *Server) Close() error {
	s.l.Lock()
	defer s.l.Unlock()
	if s.ctx == nil {
		return fmt.Errorf("not initialized")
	}
	// Stop accepting requests, unblock Serve(). Do it only once.
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			logging.WithError(err).Errorf(s.ctx, "Failed to close the listening socket")
		}
		s.listener = nil
	}
	s.cancel() // notify all running handlers to stop
	return nil
}

////////////////////////////////////////////////////////////////////////////////
// Protocol implementation.

// methodRe defines an URL of RPC method handler.
var methodRe = regexp.MustCompile(`^/rpc/LuciLocalAuthService\.([a-zA-Z0-9_]+)$`)

// minTokenLifetime is a lifetime of tokens requested through TokenGenerator.
//
// Must be larger than 'minAcceptedLifetime' in common/auth, or weird things
// may happen if local_auth server is used as a basis for some common/auth
// Authenticator.
const minTokenLifetime = 3 * time.Minute

// handle is called by http.Server in a separate goroutine to handle a request.
//
// It implements the server side of local_auth RPC protocol:
//  * Each request is POST to /rpc/LuciLocalAuthService.<Method>
//  * Request content type is "application/json; ...".
//  * The sender must set Content-Length header.
//  * Response content type is also "application/json".
//  * The server sets Content-Length header in the response.
//  * Protocol-level errors have non-200 HTTP status code.
//  * Logic errors have 200 HTTP status code and error is communicated in
//    the response body.
//
// The only supported method currently is 'GetOAuthToken':
//
//    Request body:
//    {
//      "scopes": [<string scope1>, <string scope2>, ...],
//      "secret": <string from LUCI_CONTEXT.local_auth.secret>,
//      "account_id": <ID of some account from LUCI_CONTEXT.local_auth.accounts>
//    }
//    Response body:
//    {
//      "error_code": <int, on success not set or 0>,
//      "error_message": <string, on success not set>,
//      "access_token": <string with actual token (on success)>,
//      "expiry": <int with unix timestamp in seconds (on success)>
//    }
//
// See also python counterpart of this code:
// https://github.com/luci/luci-py/blob/master/client/utils/auth_server.py
type protocolHandler struct {
	ctx    context.Context           // the parent context
	wg     *sync.WaitGroup           // used for graceful shutdown
	secret []byte                    // expected "secret" value
	tokens map[string]TokenGenerator // the actual producer of tokens (per account)
}

// protocolError triggers an HTTP reply with some non-200 status code.
type protocolError struct {
	Status  int    // HTTP status to set
	Message string // the message to put in the body
}

func (e *protocolError) Error() string {
	return fmt.Sprintf("%s (HTTP %d)", e.Message, e.Status)
}

// ServeHTTP implements the protocol marshaling logic.
func (h *protocolHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	h.wg.Add(1)
	defer h.wg.Done()

	defer paniccatcher.Catch(func(p *paniccatcher.Panic) {
		logging.Fields{
			"panic.error": p.Reason,
		}.Errorf(h.ctx, "Caught panic during handling of %q: %s\n%s", r.RequestURI, p.Reason, p.Stack)
		http.Error(rw, "Internal Server Error. See logs.", http.StatusInternalServerError)
	})

	logging.Debugf(h.ctx, "Handling %s %s", r.Method, r.RequestURI)

	if r.Method != "POST" {
		http.Error(rw, "Expecting POST", http.StatusMethodNotAllowed)
		return
	}

	// Grab <method> from /rpc/LuciLocalAuthService.<method>.
	matches := methodRe.FindStringSubmatch(r.RequestURI)
	if len(matches) != 2 {
		http.Error(rw, "Expecting /rpc/LuciLocalAuthService.<method>", http.StatusNotFound)
		return
	}
	method := matches[1]

	// The content type must be JSON, which is also the default.
	if ct := r.Header.Get("Content-Type"); ct != "" {
		baseType, _, err := mime.ParseMediaType(ct)
		if err != nil {
			http.Error(rw, fmt.Sprintf("Can't parse Content-Type: %s", err), http.StatusBadRequest)
			return
		}
		if baseType != "application/json" {
			http.Error(rw, "Expecting 'application/json' Content-Type", http.StatusBadRequest)
			return
		}
	}

	// The content length must be given and be small enough.
	if r.ContentLength < 0 || r.ContentLength >= 64*1024 {
		http.Error(rw, "Expecting 'Content-Length' header, <64Kb", http.StatusBadRequest)
		return
	}

	// Slurp the body, it's easier to deal with []byte going forward. The body is
	// tiny anyway.
	request := make([]byte, r.ContentLength)
	if _, err := io.ReadFull(r.Body, request); err != nil {
		http.Error(rw, "Can't read the request body", http.StatusBadGateway)
		return
	}

	// Route to the appropriate RPC handler.
	response, err := h.routeToImpl(method, request)

	// *protocolError are sent as HTTP errors.
	if pErr, _ := err.(*protocolError); pErr != nil {
		http.Error(rw, pErr.Message, pErr.Status)
		return
	}

	// Transient errors are returned as HTTP 500 responses.
	if transient.Tag.In(err) {
		http.Error(rw, fmt.Sprintf("Transient error - %s", err), http.StatusInternalServerError)
		return
	}

	// Fatal errors are returned as specially structured JSON responses with
	// HTTP 200 code. Replace 'response' with it.
	if err != nil {
		fatalError := rpcs.BaseResponse{
			ErrorCode:    -1,
			ErrorMessage: err.Error(),
		}
		if withCode, ok := err.(ErrorWithCode); ok && withCode.Code() != 0 {
			fatalError.ErrorCode = withCode.Code()
		}
		response = &fatalError
	}

	// Serialize the response to grab its length.
	blob, err := json.Marshal(response)
	if err != nil {
		http.Error(rw, fmt.Sprintf("Failed to serialize the response - %s", err), http.StatusInternalServerError)
		return
	}
	blob = append(blob, '\n') // for curl's sake

	// Finally write the response.
	rw.Header().Set("Content-Type", "application/json; charset=utf-8")
	rw.Header().Set("Content-Length", fmt.Sprintf("%d", len(blob)))
	rw.WriteHeader(http.StatusOK)
	if _, err := rw.Write(blob); err != nil {
		logging.WithError(err).Warningf(h.ctx, "Failed to write the response")
	}
}

// routeToImpl calls appropriate RPC method implementation.
func (h *protocolHandler) routeToImpl(method string, request []byte) (interface{}, error) {
	switch method {
	case "GetOAuthToken":
		req := &rpcs.GetOAuthTokenRequest{}
		if err := json.Unmarshal(request, req); err != nil {
			return nil, &protocolError{
				Status:  http.StatusBadRequest,
				Message: fmt.Sprintf("Not JSON body - %s", err),
			}
		}
		return h.handleGetOAuthToken(req)
	default:
		return nil, &protocolError{
			Status:  http.StatusNotFound,
			Message: fmt.Sprintf("Unknown RPC method %q", method),
		}
	}
}

////////////////////////////////////////////////////////////////////////////////
// RPC implementations.

func (h *protocolHandler) handleGetOAuthToken(req *rpcs.GetOAuthTokenRequest) (*rpcs.GetOAuthTokenResponse, error) {
	// Validate the request.
	if err := req.Validate(); err != nil {
		return nil, &protocolError{
			Status:  400,
			Message: fmt.Sprintf("Bad request: %s.", err.Error()),
		}
	}
	// TODO(vadimsh): Remove this check once it is moved into Validate().
	if req.AccountID == "" {
		return nil, &protocolError{
			Status:  400,
			Message: `Bad request: field "account_id" is required.`,
		}
	}
	if subtle.ConstantTimeCompare(h.secret, req.Secret) != 1 {
		return nil, &protocolError{
			Status:  403,
			Message: "Invalid secret.",
		}
	}
	generator := h.tokens[req.AccountID]
	if generator == nil {
		return nil, &protocolError{
			Status:  404,
			Message: fmt.Sprintf("Unrecognized account ID %q.", req.AccountID),
		}
	}

	// Dedup and sort scopes.
	scopes := stringset.New(len(req.Scopes))
	for _, s := range req.Scopes {
		scopes.Add(s)
	}
	sortedScopes := scopes.ToSlice()
	sort.Strings(sortedScopes)

	// Ask the token provider for the token. This may produce ErrorWithCode.
	tok, err := generator(h.ctx, sortedScopes, minTokenLifetime)
	if err != nil {
		return nil, err
	}
	return &rpcs.GetOAuthTokenResponse{
		AccessToken: tok.AccessToken,
		Expiry:      tok.Expiry.Unix(),
	}, nil
}
