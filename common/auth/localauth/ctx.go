// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package localauth

import (
	"time"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/lucictx"
)

// WithLocalAuth calls the callback in an environment with a local auth server.
//
// It launches the local auth server, updates LUCI_CONTEXT and calls the
// callback. Once callback returns, it stops the server.
func WithLocalAuth(ctx context.Context, srv *Server, cb func(context.Context) error) error {
	// Bind to the port, prepare "local_auth" section of new LUCI_CONTEXT.
	localAuth, err := srv.Initialize(ctx)
	if err != nil {
		logging.WithError(err).Errorf(ctx, "Failed to launch the local auth server")
		return err
	}
	defer srv.Close() // kill the socket no matter what, ignore errors
	logging.Debugf(ctx, "The local auth server is at http://127.0.0.1:%d", localAuth.RPCPort)
	newCtx, cancel := context.WithCancel(lucictx.SetLocalAuth(ctx, localAuth))

	// Launch the auth server in a background goroutine.
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := srv.Serve(); err != nil {
			logging.WithError(err).Errorf(ctx, "Unexpected error in the local auth server loop")
		}
	}()

	// Launch the callback within the new environment.
	cbErr := cb(newCtx)

	// Notify all async ops the callback could have launched that we are done.
	cancel()

	// Gracefully stop the server.
	logging.Debugf(ctx, "Stopping the local auth server...")
	if err := srv.Close(); err != nil {
		logging.WithError(err).Warningf(ctx, "Failed to close the local auth server")
	}

	// Wait for it to really die. Should be fast. Limit by timeout just in case.
	ctx, cancel = clock.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	select {
	case <-done:
		logging.Debugf(ctx, "The local auth server stopped")
	case <-ctx.Done():
		logging.WithError(ctx.Err()).Warningf(ctx, "Giving up waiting for local auth server to stop")
		if cbErr == nil {
			cbErr = ctx.Err()
		}
	}

	return cbErr
}
