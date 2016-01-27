// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prpc

import (
	"fmt"
	"net/http"
	"sort"
	"sync"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/middleware"
)

// Server is a pRPC server to serve RPC requests.
// Zero value is valid.
type Server struct {
	// CustomAuthenticator, if true, disables the forced authentication set by
	// RegisterDefaultAuth.
	CustomAuthenticator bool

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

	r.POST("/prpc/:service/:method", base(s.handle))
}

// handle handles RPCs.
// See https://godoc.org/github.com/luci/luci-go/common/prpc#hdr-Protocol
// for pRPC protocol.
func (s *Server) handle(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	serviceName := p.ByName("service")
	methodName := p.ByName("method")
	res := s.respond(c, w, r, serviceName, methodName)

	c = logging.SetFields(c, logging.Fields{
		"service": serviceName,
		"method":  methodName,
	})
	res.write(c, w)
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
