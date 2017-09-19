// Copyright 2017 The LUCI Authors.
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

package devshell

import (
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
)

var DevshellKey = "Devshell Context"

// WithContext calls the callback in an environment with a Devshell server.
func WithContext(ctx context.Context, srv *Server, cb func(context.Context) error) error {
	// Bind to the port.
	devshell, err := srv.Initialize(ctx)
	if err != nil {
		logging.WithError(err).Errorf(ctx, "Failed to launch the Devshell server")
		return err
	}
	defer srv.Close() // kill the socket no matter what, ignore errors
	logging.Debugf(ctx, "The Devshell server is at http://127.0.0.1:%d", devshell.Port)
	newCtx, cancel := context.WithCancel(context.WithValue(ctx, &DevshellKey, devshell))

	// Launch the Devshell server in a background goroutine.
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := srv.Serve(); err != nil {
			logging.WithError(err).Errorf(ctx, "Unexpected error in the Devshell server loop")
		}
	}()

	// Launch the callback within the new environment.
	cbErr := cb(newCtx)

	// Notify all async ops the callback could have launched that we are done.
	cancel()

	// Gracefully stop the server.
	logging.Debugf(ctx, "Stopping the Devshell server...")
	if err := srv.Close(); err != nil {
		logging.WithError(err).Warningf(ctx, "Failed to close the Devshell server")
	}

	// Wait for it to really die.
	ctx, cancel = clock.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	select {
	case <-done:
		logging.Debugf(ctx, "The Devshell server stopped")
	case <-ctx.Done():
		logging.WithError(ctx.Err()).Warningf(ctx, "Giving up waiting for Devshell server to stop")
		if cbErr == nil {
			cbErr = ctx.Err()
		}
	}

	return cbErr
}
