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
	"io"
	"net"
	"strconv"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/lucictx"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"
)

const (
	// DefaultPort is the TCP port that the Server listens on by default.
	DefaultPort = 52634
)

// ServerConfig defines the parameters of the server.
type ServerConfig struct {
	// Recorder is the gRPC client to the Recorder service exposed by ResultDB.
	Recorder *pb.RecorderClient

	// AuthToken is a secret token to expect from clients. If it is "" then it
	// will be randomly generated in a secure way.
	AuthToken string
	// Port is the TCP port to listen on. If 0, the server will use DefaultPort.
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
	cfg    ServerConfig
	cancel context.CancelFunc
	errC   chan error
	ln     net.Listener
}

// NewServer creates a Server value and populates optional values with defaults.
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

	s := &Server{
		cfg:  cfg,
		errC: make(chan error, 1),
	}

	return s, nil
}

// Config retrieves the ServerConfig of a previously created Server.
//
// Use this to retrieve the resolved values of unset optional fields in the
// original ServerConfig.
func (s *Server) Config() ServerConfig {
	return s.cfg
}

// ErrC returns a channel that transmits a server error.
//
// Receiving an error on this channel implies that the server has either
// stopped running or is in the process of stopping.
func (s *Server) ErrC() <-chan error {
	return s.errC
}

// Run invokes callback in a context where the server is running.
//
// The context passed to callback will be cancelled if the server encounters
// an error. The context also has the server's information exported into it.
// If callback finishes running, Run will return the error it returned.
func (s *Server) Run(ctx context.Context, callback func(context.Context) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// TODO(sajjadm): Add Export here when implemented and explain it in the function comment.
	if err := s.Start(ctx); err != nil {
		return err
	}
	defer s.Close()

	done := make(chan error)
	go func() {
		done <- callback(ctx)
	}()

	select {
	case err := <-s.errC:
		cancel()
		<-done
		return err
	case err := <-done:
		return err
	}
}

// Start runs the server.
//
// On success, Start will return nil, and a subsequent error will be sent on
// the server's ErrC channel.
func (s *Server) Start(ctx context.Context) error {
	if s.ln != nil {
		return errors.Reason("cannot call Start twice").Err()
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", s.cfg.Port))
	if err != nil {
		return err
	}
	s.ln = ln

	_, port, err := net.SplitHostPort(s.ln.Addr().String())
	if err != nil {
		return err
	}
	s.cfg.Port, err = strconv.Atoi(port)
	if err != nil {
		return err
	}

	ctx, s.cancel = context.WithCancel(ctx)
	go s.serveLoop(ctx)
	return nil
}

func (s *Server) serveLoop(ctx context.Context) {
	defer s.ln.Close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for {
		switch conn, err := s.ln.Accept(); {
		case err == nil:
			go func() {
				if err := s.handleConnection(ctx, conn); err != nil {
					logging.Errorf(ctx, "%s", err)
				}
			}()
		case ctx.Err() != nil:
			s.errC <- ctx.Err()
			return
		case shouldKeepTrying(err):
			continue
		default:
			s.errC <- errors.Annotate(err, "unrecoverable listener error").Err()
			return
		}
	}
}

// Close immediately stops the server from accepting new connections and cancels existing ones.
func (s *Server) Close() error {
	// TODO(sajjadm): Determine if some pending connections were forcefully closed by s.cancel()
	// and report that as an error.
	s.cancel()
	return s.ln.Close()
}

// Process handles a message as if it had been sent over the TCP interface.
func (s *Server) Process(msg *sinkpb.SinkMessageContainer) error {
	return errors.New("not implemented yet")
}

// Export exports lucictx.ResultDB derived from the server configuration into
// the context.
func (s *Server) Export(ctx context.Context) context.Context {
	db := lucictx.ResultDB{
		TestResults: lucictx.TestResults{Port: s.cfg.Port, AuthToken: s.cfg.AuthToken},
	}
	return lucictx.SetResultDB(ctx, &db)
}

func (s *Server) handleConnection(ctx context.Context, c net.Conn) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-ctx.Done()
		c.Close()
	}()

	dc := json.NewDecoder(c)
	if err := processHandshake(dc, s.cfg.AuthToken); err != nil {
		return errors.Annotate(err, "handshake failed").Err()
	}
	logging.Debugf(ctx, "Successful handshake")

	if err := processMessages(dc); err != nil && err != io.EOF {
		return err
	}

	return nil
}

func processMessages(dc *json.Decoder) error {
	for {
		msgp := &sinkpb.SinkMessageContainer{}
		if err := readMessage(dc, msgp); err != nil {
			return errors.Annotate(err, "failed to read message").Err()
		}

		// TODO(sajjadm): msgp is valid, do something with it
	}
}

func readMessage(dc *json.Decoder, dest proto.Message) error {
	for {
		err := jsonpb.UnmarshalNext(dc, dest)
		if shouldKeepTrying(err) {
			continue
		}
		return err
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
