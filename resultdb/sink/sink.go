// Copyright 2019 The LUCI Authors.
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

// Package sink provides a server for aggregating test results and sending them
// to the ResultDB backend.
package sink

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/golang/protobuf/jsonpb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"
)

const (
	DefaultPort = 52634
)

// ServerConfig defines the parameters of the server.
type ServerConfig struct {
	// Recorder is the gRPC client to the Recorder service exposed by ResultDB.
	Recorder *pb.RecorderClient

	// AuthToken is a secret token to expect from clients. Optional.
	AuthToken string
	// Port is the TCP port to listen on. Optional.
	Port int

	// Invocation is the name of the invocation that test results should append
	// to.
	Invocation string
	// UpdateToken is the token that allows writes to Invocation.
	UpdateToken string

	// TestPathPrefix will be prepended to the test_path of each TestResult.
	TestPathPrefix string
}

// Server contains state relevant to the server itself.
// It should always be created by a call to NewServer.
// After a call to Serve(), Server will accept connections on its Port and
// gather test results to send to its Recorder.
type Server struct {
	cfg             ServerConfig
	shutdownStarted int32 // Only access with atomic functions
	cancel          context.CancelFunc
	ready           chan struct{}
	ln              net.Listener
}

// NewServer creates a Server value and populates optional values with defaults.
//
// If cfg.AuthToken is "" it will be randomly generated in a secure way.
// If cfg.Port is 0 then DefaultPort will be used.
func NewServer(ctx context.Context, cfg ServerConfig) (*Server, error) {
	if cfg.AuthToken == "" {
		buf := make([]byte, 32)
		if _, err := cryptorand.Read(ctx, buf); err != nil {
			return nil, err
		}
		cfg.AuthToken = hex.EncodeToString(buf)
	}

	if cfg.Port == 0 {
		cfg.Port = DefaultPort
	}

	s := &Server{cfg: cfg}
	s.ready = make(chan struct{})
	return s, nil
}

// Config retrieves the ServerConfig of a previously created Server.
//
// Use this to retrieve the resolved values of unset optional fields in the
// original ServerConfig.
func (s *Server) Config() ServerConfig {
	return s.cfg
}

// Ready retrieves the Server's ready channel.
//
// The channel is closed once the Server is listening and otherwise
// has no messages.
func (s *Server) Ready() chan struct{} {
	return s.ready
}

// Serve runs the Server in a background goroutine.
//
// The Server will continue to run until a subsequent call to Close.
// If ctx is cancelled the background goroutine will stop accepting connections
// and terminate.
func (s *Server) Serve(ctx context.Context) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", s.cfg.Port))
	s.ln = ln
	if err != nil {
		return err
	}

	_, port, err := net.SplitHostPort(s.ln.Addr().String())
	if err != nil {
		return err
	}
	s.cfg.Port, err = strconv.Atoi(port)
	if err != nil {
		return err
	}

	ctx, s.cancel = context.WithCancel(ctx)
	close(s.ready)
	return s.serveLoop(ctx, s.ln.(*net.TCPListener))
}

func (s *Server) serveLoop(ctx context.Context, ln *net.TCPListener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if atomic.LoadInt32(&s.shutdownStarted) == 1 {
				return errors.Reason("server was closed").Err()
			}
			if err := ctx.Err(); err != nil {
				return errors.Annotate(err, "context was cancelled").Err()
			}
			if shouldKeepTrying(err) {
				continue
			}
			return errors.Annotate(err, "unrecoverable listener error").Err()
		}

		go s.handleConnection(ctx, conn)
	}
}

// Close immediately closes the net.Listener and cancels all connection handlers.
func (s *Server) Close(ctx context.Context) error {
	atomic.StoreInt32(&s.shutdownStarted, 1)
	err := s.ln.Close()
	s.cancel()
	return err
}

// Process handles a message as if it had been sent over the TCP interface.
func (s *Server) Process(msg *sinkpb.SinkMessageContainer) error {
	return errors.New("not implemented yet")
}

// Export exports lucictx.ResultSink derived from the server configuration into
// the context.
// TODO(crbug.com/1017288) lucictx.ResultSink does not exist yet.
func (s *Server) Export(ctx context.Context) context.Context {
	return nil
}

func (s *Server) handleConnection(ctx context.Context, c net.Conn) {
	defer c.Close()
	dc := json.NewDecoder(c)
	if err := processHandshake(dc, s.cfg.AuthToken); err != nil {
		logging.Errorf(ctx, "handshake failed: %s", err)
		return
	}
	logging.Debugf(ctx, "Successful handshake")

	// TODO(crbug.com/1017288) Actually use msg later.
	var msg sinkpb.SinkMessageContainer
	for {
		if ctx.Err() != nil {
			return
		}

		c.SetDeadline(clock.Now(ctx).Add(500 * time.Millisecond))
		if err := jsonpb.UnmarshalNext(dc, &msg); err != nil {
			if shouldKeepTrying(err) {
				continue
			}
			logging.Errorf(ctx, "reading next message failed: %s", err)
			return
		}
	}
}

func processHandshake(dc *json.Decoder, authToken string) error {
	var hs sinkpb.Handshake
	if err := jsonpb.UnmarshalNext(dc, &hs); err != nil {
		return errors.Reason("failed to parse Handshake").Err()
	}
	if hs.GetAuthToken() != authToken {
		return errors.Reason("handshake message had invalid AuthToken").Err()
	}
	return nil
}

func shouldKeepTrying(err error) bool {
	e, ok := err.(net.Error)
	return ok && (e.Temporary() || e.Timeout())
}
