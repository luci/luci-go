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
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/golang/protobuf/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
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

// AllowOriginAll returns true unconditionally.
// It can be used as Server.AccessControl.
// It must NOT be used in combination with cookie-based authentication.
func AllowOriginAll(c context.Context, origin string) bool {
	return true
}

// Override is a pRPC method-specific override which may optionally handle the
// entire pRPC method call. If it returns true, the override is assumed to
// have fully handled the pRPC method call and processing of the request does
// not continue. In this case it's the override's responsibility to adhere to
// all pRPC semantics. However if it returns false, processing continues as
// normal, allowing the override to act as a preprocessor. In this case it's
// the override's responsibility to ensure it hasn't done anything that will
// be incompatible with pRPC semantics (such as writing garbage to the response
// writer in the router context).
type Override func(*router.Context) bool

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

	// HackFixFieldMasksForJSON indicates whether to attempt a workaround for
	// https://github.com/golang/protobuf/issues/745 when the request has
	// Content-Type: application/json. This hack is scheduled for removal.
	// TODO(crbug/1082369): Remove this workaround once field masks can be decoded.
	HackFixFieldMasksForJSON bool

	// UnaryServerInterceptor provides a hook to intercept the execution of
	// a unary RPC on the server. It is the responsibility of the interceptor to
	// invoke handler to complete the RPC.
	UnaryServerInterceptor grpc.UnaryServerInterceptor

	mu        sync.RWMutex
	services  map[string]*service
	overrides map[string]map[string]Override
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

// RegisterOverride registers an overriding function.
//
// Panics if an override for the given service method is already registered.
func (s *Server) RegisterOverride(serviceName, methodName string, fn Override) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.overrides == nil {
		s.overrides = map[string]map[string]Override{}
	}
	if _, ok := s.overrides[serviceName]; !ok {
		s.overrides[serviceName] = map[string]Override{}
	}
	if _, ok := s.overrides[serviceName][methodName]; ok {
		panic(fmt.Errorf("method %q of service %q is already overridden", methodName, serviceName))
	}

	s.overrides[serviceName][methodName] = fn
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
		ctx, err := a.Authenticate(c.Context, c.Request)
		if err == nil {
			c.Context = ctx
			next(c)
			return
		}

		format, perr := responseFormat(c.Request.Header.Get(headerAccept))
		if perr != nil {
			writeError(c.Context, c.Writer, perr, FormatBinary)
			return
		}

		// Authenticate is allowed to return gRPC-tagger errors, recognize them.
		code, ok := grpcutil.Tag.In(err)
		if !ok {
			if transient.Tag.In(err) {
				code = codes.Internal
			} else {
				code = codes.Unauthenticated
			}
		}
		writeError(c.Context, c.Writer, withCode(err, code), format)
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

	override, service, method, methodFound := s.lookup(serviceName, methodName)
	// Override takes precedence over notImplementedErr.
	if override != nil && override(c) {
		return
	}

	s.setAccessControlHeaders(c, false)
	c.Writer.Header().Set("X-Content-Type-Options", "nosniff")

	res := response{}
	switch {
	case service == nil:
		res.err = status.Errorf(
			codes.Unimplemented,
			"service %q is not implemented",
			serviceName)
	case !methodFound:
		res.err = status.Errorf(
			codes.Unimplemented,
			"method %q in service %q is not implemented",
			methodName, serviceName)
	default:
		s.call(c, service, method, &res)
	}

	if res.err != nil {
		writeError(c.Context, c.Writer, res.err, res.fmt)
		return
	}
	writeMessage(c.Context, c.Writer, res.out, res.fmt)
}

func (s *Server) handleOPTIONS(c *router.Context) {
	s.setAccessControlHeaders(c, true)
	c.Writer.WriteHeader(http.StatusOK)
}

var requestContextKey = "context key with *requestContext"

type requestContext struct {
	// additional headers that will be sent in the response
	header http.Header
}

// SetHeader sets the header metadata.
// When called multiple times, all the provided metadata will be merged.
//
// If ctx is not a pRPC server context, then SetHeader calls grpc.SetHeader
// such that calling prpc.SetHeader works for both pRPC and gRPC.
func SetHeader(ctx context.Context, md metadata.MD) error {
	if rctx, ok := ctx.Value(&requestContextKey).(*requestContext); ok {
		for k, vs := range md {
			if strings.HasPrefix(k, "X-Prpc-") || k == headerContentType {
				return errors.Reason("reserved header key %q", k).Err()
			}
			for _, v := range vs {
				rctx.header.Add(metaToHeader(k, v))
			}
		}
		return nil
	}

	return grpc.SetHeader(ctx, md)
}

type response struct {
	out proto.Message
	fmt Format
	err error
}

func (s *Server) lookup(serviceName, methodName string) (override Override, service *service, method grpc.MethodDesc, methodFound bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if methods, ok := s.overrides[serviceName]; ok {
		override = methods[methodName]
	}
	service = s.services[serviceName]
	if service == nil {
		return
	}
	method, methodFound = service.methods[methodName]
	return
}

func (s *Server) call(c *router.Context, service *service, method grpc.MethodDesc, r *response) {
	var perr *protocolError
	r.fmt, perr = responseFormat(c.Request.Header.Get(headerAccept))
	if perr != nil {
		r.err = perr
		return
	}

	methodCtx, err := parseHeader(c.Context, c.Request.Header, c.Request.Host)
	if err != nil {
		r.err = withStatus(err, http.StatusBadRequest)
		return
	}

	methodCtx = context.WithValue(methodCtx, &requestContextKey, &requestContext{header: c.Writer.Header()})

	out, err := method.Handler(service.impl, methodCtx, func(in interface{}) error {
		if in == nil {
			return grpcutil.Errf(codes.Internal, "input message is nil")
		}
		// Do not collapse it to one line. There is implicit err type conversion.
		if perr := readMessage(c.Request, in.(proto.Message), s.HackFixFieldMasksForJSON); perr != nil {
			return perr
		}
		return nil
	}, s.UnaryServerInterceptor)

	switch {
	case err != nil:
		r.err = err
	case out == nil:
		r.err = status.Error(codes.Internal, "service returned nil message")
	default:
		r.out = out.(proto.Message)
	}
	return
}

func (s *Server) setAccessControlHeaders(c *router.Context, preflight bool) {
	// Don't write out access control headers if the origin is unspecified.
	const originHeader = "Origin"
	origin := c.Request.Header.Get(originHeader)
	if origin == "" || s.AccessControl == nil || !s.AccessControl(c.Context, origin) {
		return
	}

	h := c.Writer.Header()
	h.Add("Access-Control-Allow-Origin", origin)
	h.Add("Vary", originHeader)
	h.Add("Access-Control-Allow-Credentials", "true")

	if preflight {
		h.Add("Access-Control-Allow-Headers", allowHeaders)
		h.Add("Access-Control-Allow-Methods", allowMethods)
		h.Add("Access-Control-Max-Age", allowPreflightCacheAgeSecs)
	} else {
		h.Add("Access-Control-Expose-Headers", exposeHeaders)
	}
}

// ServiceNames returns a sorted list of full names of all registered services.
func (s *Server) ServiceNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.services))
	for name := range s.services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
