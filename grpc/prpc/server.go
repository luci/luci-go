// Copyright 2016 The LUCI Authors.
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

package prpc

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/router"
)

var (
	// Describe the permitted Access Control requests.
	allowHeaders = strings.Join([]string{"Origin", "Content-Type", "Accept", "Authorization"}, ", ")
	allowMethods = strings.Join([]string{"OPTIONS", "POST"}, ", ")

	// allowPreflightCacheAgeSecs is the amount of time to enable the browser to
	// cache the preflight access control response, in seconds.
	//
	// 600 seconds is 10 minutes.
	allowPreflightCacheAgeSecs = "600"

	// exposeHeaders lists the whitelisted non-standard response headers that the
	// client may accept.
	exposeHeaders = strings.Join([]string{HeaderGRPCCode}, ", ")

	// NoAuthentication can be used in place of an Authenticator to explicitly
	// specify that your Server will skip authentication.
	//
	// Use it with Server.Authenticator or RegisterDefaultAuth.
	NoAuthentication Authenticator = nullAuthenticator{}
)

// Server is a pRPC server to serve RPC requests.
// Zero value is valid.
type Server struct {
	// Authenticator, if not nil, specifies how to authenticate requests.
	//
	// If nil, the default authenticator set by RegisterDefaultAuth will be used.
	// If the default authenticator is also nil, all request handlers will panic.
	//
	// If you want to disable the authentication (e.g for tests), explicitly set
	// Authenticator to NoAuthentication.
	Authenticator Authenticator

	// AccessControl, if not nil, is a callback that is invoked per request to
	// determine if permissive access control headers should be added to the
	// response.
	//
	// This callback includes the request Context and the origin header supplied
	// by the client. If nil, or if it returns false, no headers will be written.
	// Otherwise, access control headers for the specified origin will be
	// included in the response.
	AccessControl func(c context.Context, origin string) bool

	// UnaryServerInterceptor provides a hook to intercept the execution of
	// a unary RPC on the server. It is the responsibility of the interceptor to
	// invoke handler to complete the RPC.
	UnaryServerInterceptor grpc.UnaryServerInterceptor

	mu       sync.Mutex
	services map[string]*service
}

type service struct {
	methods map[string]grpc.MethodDesc
	impl    interface{}
}

// RegisterService registers a service implementation.
// Called from the generated code.
//
// desc must contain description of the service, its message types
// and all transitive dependencies.
//
// Panics if a service of the same name is already registered.
func (s *Server) RegisterService(desc *grpc.ServiceDesc, impl interface{}) {
	serv := &service{
		impl:    impl,
		methods: make(map[string]grpc.MethodDesc, len(desc.Methods)),
	}
	for _, m := range desc.Methods {
		serv.methods[m.MethodName] = m
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.services == nil {
		s.services = map[string]*service{}
	} else if _, ok := s.services[desc.ServiceName]; ok {
		panic(fmt.Errorf("service %q is already registered", desc.ServiceName))
	}

	s.services[desc.ServiceName] = serv
}

// authenticate forces authentication set by RegisterDefaultAuth.
func (s *Server) authenticate() router.Middleware {
	a := s.Authenticator
	if a == nil {
		a = GetDefaultAuth()
		if a == nil {
			panic("prpc: no custom Authenticator was provided and default authenticator was not registered.\n" +
				"Either explicitly set `Server.Authenticator = NoAuthentication`, or use RegisterDefaultAuth()")
		}
	}

	return func(c *router.Context, next router.Handler) {
		switch ctx, err := a.Authenticate(c.Context, c.Request); {
		case transient.Tag.In(err):
			res := errResponse(codes.Internal, http.StatusInternalServerError, "%s", err)
			res.write(c.Context, c.Writer)
		case err != nil:
			res := errResponse(codes.Unauthenticated, http.StatusUnauthorized, "%s", err)
			res.write(c.Context, c.Writer)
		default:
			c.Context = ctx
			next(c)
		}
	}
}

// InstallHandlers installs HTTP handlers at /prpc/:service/:method.
//
// See https://godoc.org/go.chromium.org/luci/grpc/prpc#hdr-Protocol
// for pRPC protocol.
//
// The authenticator in 'base' is always replaced by pRPC specific one. For more
// details about the authentication see Server.Authenticator doc.
func (s *Server) InstallHandlers(r *router.Router, base router.MiddlewareChain) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rr := r.Subrouter("/prpc/:service/:method")
	rr.Use(base.Extend(s.authenticate()))

	rr.POST("", router.MiddlewareChain{}, s.handlePOST)
	rr.OPTIONS("", router.MiddlewareChain{}, s.handleOPTIONS)
}

// handle handles RPCs.
// See https://godoc.org/go.chromium.org/luci/grpc/prpc#hdr-Protocol
// for pRPC protocol.
func (s *Server) handlePOST(c *router.Context) {
	serviceName := c.Params.ByName("service")
	methodName := c.Params.ByName("method")
	res := s.respond(c.Context, c.Writer, c.Request, serviceName, methodName)

	s.setAccessControlHeaders(c.Context, c.Request, c.Writer, false)
	res.write(c.Context, c.Writer)
}

func (s *Server) handleOPTIONS(c *router.Context) {
	s.setAccessControlHeaders(c.Context, c.Request, c.Writer, true)
	c.Writer.WriteHeader(http.StatusOK)
}

func (s *Server) respond(c context.Context, w http.ResponseWriter, r *http.Request, serviceName, methodName string) *response {
	service := s.services[serviceName]
	if service == nil {
		return errResponse(
			codes.Unimplemented,
			http.StatusNotImplemented,
			"service %q is not implemented",
			serviceName)
	}

	method, ok := service.methods[methodName]
	if !ok {
		return errResponse(
			codes.Unimplemented,
			http.StatusNotImplemented,
			"method %q in service %q is not implemented",
			methodName,
			serviceName)
	}

	format, perr := responseFormat(r.Header.Get(headerAccept))
	if perr != nil {
		return respondProtocolError(perr)
	}

	c, err := parseHeader(c, r.Header)
	if err != nil {
		return respondProtocolError(withStatus(err, http.StatusBadRequest))
	}

	out, err := method.Handler(service.impl, c, func(in interface{}) error {
		if in == nil {
			return grpcutil.Errf(codes.Internal, "input message is nil")
		}
		// Do not collapse it to one line. There is implicit err type conversion.
		if perr := readMessage(r, in.(proto.Message)); perr != nil {
			return perr
		}
		return nil
	}, s.UnaryServerInterceptor)
	if err != nil {
		if perr, ok := err.(*protocolError); ok {
			return respondProtocolError(perr)
		}
		return errResponse(errorCode(err), 0, "%s", grpc.ErrorDesc(err))
	}

	if out == nil {
		return errResponse(codes.Internal, 0, "service returned nil message")
	}
	return respondMessage(out.(proto.Message), format)
}

func (s *Server) setAccessControlHeaders(c context.Context, r *http.Request, w http.ResponseWriter, preflight bool) {
	// Don't write out access control headers if the origin is unspecified.
	const originHeader = "Origin"
	origin := r.Header.Get(originHeader)
	if origin == "" || s.AccessControl == nil || !s.AccessControl(c, origin) {
		return
	}

	w.Header().Add("Access-Control-Allow-Origin", origin)
	w.Header().Add("Vary", originHeader)
	w.Header().Add("Access-Control-Allow-Credentials", "true")

	if preflight {
		w.Header().Add("Access-Control-Allow-Headers", allowHeaders)
		w.Header().Add("Access-Control-Allow-Methods", allowMethods)
		w.Header().Add("Access-Control-Max-Age", allowPreflightCacheAgeSecs)
	} else {
		w.Header().Add("Access-Control-Expose-Headers", exposeHeaders)
	}
}

// ServiceNames returns a sorted list of full names of all registered services.
func (s *Server) ServiceNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	names := make([]string, 0, len(s.services))
	for name := range s.services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
