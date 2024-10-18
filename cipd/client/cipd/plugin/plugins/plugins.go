// Copyright 2020 The LUCI Authors.
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

// Package plugins contains shared plugin-side functionality.
package plugins

import (
	"context"
	"fmt"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cipd/client/cipd/plugin/protocol"
)

// RunLoop executes some concrete plugin logic.
//
// It should run some kind of a loop until the context is canceled. It happens
// when the host decides to terminate the plugin.
//
// Any logging done on this context goes to the host.
type RunLoop func(context.Context, *grpc.ClientConn) error

// Run connects to the host and calls `run`.
//
// Blocks until the stdin closes (which indicates the plugin should terminate).
func Run(ctx context.Context, stdin io.ReadCloser, run RunLoop) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Receives the handshake message once it is read from the stdin.
	type handshakeAndErr struct {
		PB *protocol.Handshake
		error
	}
	handshakeCh := make(chan handshakeAndErr, 1)

	// Read stdin in a goroutine to be able to cancel it on `ctx` deadline.
	stdinDone := make(chan struct{})
	go func() {
		defer close(stdinDone)

		// Read the handshake message.
		handshake, err := readHandshake(stdin)
		handshakeCh <- handshakeAndErr{handshake, err}

		// Wait until the host decides to terminate the plugin by closing the stdin.
		io.Copy(io.Discard, stdin)
	}()
	defer func() { <-stdinDone }() // don't leak the goroutine on return

	// Cancel everything when the stdin closes.
	go func() {
		select {
		case <-stdinDone:
			cancel()
		case <-ctx.Done():
		}
	}()

	// Unblock all above goroutines by forcefully closing `stdin` if `ctx` is
	// canceled via its parent.
	go func() {
		<-ctx.Done()
		stdin.Close()
	}()

	// All prep for comfortably reading the stdin with timeouts is done. Now do
	// the actual handshake.

	handshake := <-handshakeCh
	if handshake.error != nil {
		return errors.Annotate(handshake.error, "failed to read the handshake message").Err()
	}
	conn, err := grpc.NewClient(
		fmt.Sprintf("passthrough:///127.0.0.1:%d", handshake.PB.Port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&pluginPerRPCCredentials{handshake.PB.Ticket}),
	)
	if err != nil {
		return errors.Annotate(err, "failed to connect to the plugin host").Err()
	}
	defer conn.Close()

	// A context that sends logging messages to the host's log.
	logCtx := logging.SetFactory(ctx, func(lctx context.Context) logging.Logger {
		return &hostLogger{
			lctx: lctx,
			rctx: ctx,
			host: protocol.NewHostClient(conn),
		}
	})

	return run(logCtx, conn)
}

////////////////////////////////////////////////////////////////////////////////

// readHandshake reads and deserializes the handshake message.
func readHandshake(stdin io.Reader) (*protocol.Handshake, error) {
	buf := make([]byte, 0, 200)

	// Carefully read byte-by-byte to avoid getting stuck trying to buffer more
	// data than available.
	b := [1]byte{0}
	for b[0] != '\n' {
		if _, err := stdin.Read(b[:]); err != nil {
			return nil, err
		}
		buf = append(buf, b[0])
	}

	var handshake protocol.Handshake
	if err := protojson.Unmarshal(buf, &handshake); err != nil {
		return nil, err
	}
	return &handshake, nil
}

////////////////////////////////////////////////////////////////////////////////

// pluginPerRPCCredentials implements credentials.PerRPCCredentials.
type pluginPerRPCCredentials struct {
	ticket string
}

func (c *pluginPerRPCCredentials) RequireTransportSecurity() bool {
	return false // we are on a localhost
}

func (c *pluginPerRPCCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{"x-plugin-ticket": c.ticket}, nil
}

////////////////////////////////////////////////////////////////////////////////

// hostLogger sends log messages to the plugin host.
type hostLogger struct {
	lctx context.Context // a context at the logging site
	rctx context.Context // a context for making RPCs
	host protocol.HostClient
}

func (l *hostLogger) Debugf(format string, args ...any) {
	l.LogCall(logging.Debug, 1, format, args)
}

func (l *hostLogger) Infof(format string, args ...any) {
	l.LogCall(logging.Info, 1, format, args)
}

func (l *hostLogger) Warningf(format string, args ...any) {
	l.LogCall(logging.Warning, 1, format, args)
}

func (l *hostLogger) Errorf(format string, args ...any) {
	l.LogCall(logging.Error, 1, format, args)
}

func (l *hostLogger) LogCall(lvl logging.Level, calldepth int, format string, args []any) {
	if logging.IsLogging(l.lctx, lvl) {
		msg := fmt.Sprintf(format, args...)
		if fields := logging.GetFields(l.lctx); len(fields) != 0 {
			msg += " :: " + fields.String()
		}
		l.host.Log(l.rctx, &protocol.LogRequest{
			Severity: lvl.String(),
			Message:  msg,
		})
	}
}
