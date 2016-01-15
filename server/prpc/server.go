// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prpc

import (
	"sort"
	"sync"

	"github.com/julienschmidt/httprouter"
	"google.golang.org/grpc"

	"github.com/luci/luci-go/server/middleware"
)

// Server is a pRPC server to serve RPC requests.
// Zero value is valid.
type Server struct {
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
		panicf("service %q is already registered", desc.ServiceName)
	}

	s.services[desc.ServiceName] = serv
}

// InstallHandlers installs HTTP handlers at /prpc/{service_name}/{method_name}
// for all registered services.
//
// All service methods accept POST requests.
// Those whose input message has no fields, accept GET requests as well.
func (s *Server) InstallHandlers(r *httprouter.Router, base middleware.Base) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, service := range s.services {
		for _, m := range service.methods {
			m.InstallHandlers(r, base)
		}
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
