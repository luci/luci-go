// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prpc

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/middleware"

	prpccommon "github.com/luci/luci-go/common/prpc"
)

var (
	// Describe the permitted Access Control requests.
	allowHeaders = strings.Join([]string{"Origin", "Content-Type", "Accept"}, ", ")
	allowMethods = strings.Join([]string{"OPTIONS", "POST"}, ", ")

	// allowPreflightCacheAgeSecs is the amount of time to enable the browser to
	// cache the preflight access control response, in seconds.
	//
	// 600 seconds is 10 minutes.
	allowPreflightCacheAgeSecs = "600"

	// exposeHeaders lists the whitelisted non-standard response headers that the
	// client may accept.
	exposeHeaders = strings.Join([]string{prpccommon.HeaderGRPCCode}, ", ")
)

// Server is a pRPC server to serve RPC requests.
// Zero value is valid.
type Server struct {
	// CustomAuthenticator, if true, disables the forced authentication set by
	// RegisterDefaultAuth.
	CustomAuthenticator bool

	// AccessControl, if not nil, is a callback that is invoked per request to
	// determine if permissive access control headers should be added to the
	// response.
	//
	// This callback includes the request Context and the origin header supplied
	// by the client. If nil, or if it returns false, no headers will be written.
	// Otherwise, access control headers for the specified origin will be
	// included in the response.
	AccessControl func(c context.Context, origin string) bool

	mu       sync.Mutex
	services map[string]*service
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
		desc:    desc,
		impl:    impl,
		methods: make(map[string]*method, len(desc.Methods)),
	}

	for _, grpcDesc := range desc.Methods {
		serv.methods[grpcDesc.MethodName] = &method{
			service: serv,
			desc:    grpcDesc,
		}
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
func (s *Server) authenticate(base middleware.Base) middleware.Base {
	a := GetDefaultAuth()
	if a == nil {
		panic("prpc: CustomAuthenticator is false, but default authenticator was not registered. " +
			"Forgot to import appengine/gaeauth/server package?")
	}

	return func(h middleware.Handler) httprouter.Handle {
		return base(func(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
			c = auth.SetAuthenticator(c, a)
			c, err := a.Authenticate(c, r)
			if err != nil {
				res := errResponse(codes.Unauthenticated, http.StatusUnauthorized, escapeFmt(err.Error()))
				res.write(c, w)
				return
			}
			h(c, w, r, p)
		})
	}
}

// InstallHandlers installs HTTP handlers at /prpc/:service/:method.
// See https://godoc.org/github.com/luci/luci-go/common/prpc#hdr-Protocol
// for pRPC protocol.
func (s *Server) InstallHandlers(r *httprouter.Router, base middleware.Base) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.CustomAuthenticator {
		base = s.authenticate(base)
	}

	r.POST("/prpc/:service/:method", base(s.handlePOST))
	r.OPTIONS("/prpc/:service/:method", base(s.handleOPTIONS))
}

// handle handles RPCs.
// See https://godoc.org/github.com/luci/luci-go/common/prpc#hdr-Protocol
// for pRPC protocol.
func (s *Server) handlePOST(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	serviceName := p.ByName("service")
	methodName := p.ByName("method")
	res := s.respond(c, w, r, serviceName, methodName)

	c = logging.SetFields(c, logging.Fields{
		"service": serviceName,
		"method":  methodName,
	})
	s.setAccessControlHeaders(c, r, w, false)
	res.write(c, w)
}

func (s *Server) handleOPTIONS(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	s.setAccessControlHeaders(c, r, w, true)
	w.WriteHeader(http.StatusOK)
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

	method := service.methods[methodName]
	if method == nil {
		return errResponse(
			codes.Unimplemented,
			http.StatusNotImplemented,
			"method %q in service %q is not implemented",
			methodName,
			serviceName)
	}

	return method.handle(c, w, r)
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
