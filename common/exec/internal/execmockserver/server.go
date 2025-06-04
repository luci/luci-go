// Copyright 2023 The LUCI Authors.
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

// Package execmockserver implements the "net/rpc" based client/server pair
// which is used by execmock to communicate to/from mock subprocesses.
package execmockserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/rpc"
	"reflect"
	"sync"
	"time"

	"go.chromium.org/luci/common/errors"
)

// execServeEnvvar is the environment variable used for launching the execmock
// subprocess.
//
// Its value will be host:port, plus an invocation ID like:
//
//	localhost:????|<invocation_id>
//
// This host:port will be a `net/rpc` server for the "execMockServer". The
// invocation_id will be an integer used for interacting with the
// ExecMockServer.
const execServeEnvvar = "LUCI_EXECMOCK_CTX"

type InvocationInput struct {
	RunnerID    uint64
	RunnerInput any
	StartError  error
}

type InvocationOutput struct {
	InvocationID uint64
	RunnerOutput any
	RunnerError  string
	RunnerPanic  string
}

type Server struct {
	mu sync.Mutex

	hostname string

	invocationID uint64
	inputs       map[uint64]*InvocationInput
	outputs      map[uint64]func(any, string, error)

	server *http.Server
}

func (e *Server) GetInvocationInput(invocationID uint64, reply *InvocationInput) error {
	e.mu.Lock()
	inp := e.inputs[invocationID]
	delete(e.inputs, invocationID)
	e.mu.Unlock()

	if inp == nil {
		return errors.Fmt("Bad invocation ID: %d", invocationID)
	}

	*reply = *inp
	return nil
}

func (e *Server) SetInvocationOutput(InvocationOutput *InvocationOutput, none *int) error {
	*none = 0

	e.mu.Lock()
	outfn := e.outputs[InvocationOutput.InvocationID]
	delete(e.outputs, InvocationOutput.InvocationID)
	e.mu.Unlock()

	if outfn == nil {
		return errors.Fmt("Bad invocation ID: %d", InvocationOutput.InvocationID)
	}

	var runnerErr error
	if InvocationOutput.RunnerError != "" {
		runnerErr = errors.New(InvocationOutput.RunnerError)
	}
	outfn(InvocationOutput.RunnerOutput, InvocationOutput.RunnerPanic, runnerErr)
	return nil
}

func (e *Server) Close() error {
	ctx, closer := context.WithTimeout(context.Background(), time.Second)
	defer closer()
	return e.server.Shutdown(ctx)
}

// RegisterInvocation registers an invocation and returns an environment
// variable which can be set for a subprocess which calls `Intercept()`.
func RegisterInvocation[Out any](s *Server, InvocationInput *InvocationInput, outputFn func(Out, string, error)) (string, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.invocationID++
	id := s.invocationID

	s.inputs[id] = InvocationInput
	s.outputs[id] = func(data any, panicStack string, err error) {
		var o Out
		oT := reflect.TypeOf(&o).Elem()
		if oT.Kind() == reflect.Pointer {
			tmp := reflect.New(oT.Elem())
			tmp.Elem().Set(reflect.ValueOf(data))
			outputFn(tmp.Interface().(Out), panicStack, err)
		} else {
			outputFn(data.(Out), panicStack, err)
		}
	}

	return fmt.Sprintf("%s=%s|%d", execServeEnvvar, s.hostname, id), id
}

func Start() (ret *Server) {
	ret = &Server{
		inputs:  map[uint64]*InvocationInput{},
		outputs: map[uint64]func(any, string, error){},
	}

	srv := rpc.NewServer()
	if err := srv.RegisterName("ExecMockServer", ret); err != nil {
		panic(err)
	}

	l, err := net.Listen("tcp", "localhost:")
	if err != nil {
		panic(err)
	}
	ret.hostname = l.Addr().String()
	mux := http.NewServeMux()
	mux.Handle(rpc.DefaultRPCPath, srv)

	ret.server = &http.Server{Handler: mux}
	go ret.server.Serve(l)

	return ret
}
