// Copyright 2021 The LUCI Authors.
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

// Package plugin contains public API of the plugin system.
package plugin

import (
	"context"

	"google.golang.org/grpc"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/common"
)

// Config is used to initialize the plugin host.
type Config struct {
	ServiceURL string           // URL of the CIPD repository ("https://...") used by the client
	Repository RepositoryClient // a subset of repopb.RepositoryClient available to plugins
}

// RepositoryClient is a subset of repopb.RepositoryClient available to plugins.
type RepositoryClient interface {
	// Lists metadata entries attached to an instance.
	ListMetadata(ctx context.Context, in *repopb.ListMetadataRequest, opts ...grpc.CallOption) (*repopb.ListMetadataResponse, error)
}

// Host is used by the CIPD client to launch and communicate with plugins.
//
// Use host.Host for the production implementation that uses out-of-process
// plugins.
type Host interface {
	// Initialize is called when the CIPD client starts before any other call.
	Initialize(cfg Config) error

	// Close is called when the CIPD client closes.
	Close(ctx context.Context)

	// NewAdmissionPlugin returns a handle to an admission plugin.
	//
	// The returned AdmissionPlugin can be used right away to enqueue admission
	// checks. The plugin subprocess will lazily be started on the first
	// CheckAdmission call. All enqueued checks will eventually be processed by
	// the plugin or rejected if the plugin fails to start.
	NewAdmissionPlugin(cmdLine []string) (AdmissionPlugin, error)
}

// AdmissionPlugin is used by the CIPD client to check if it is OK to deploy
// a package.
type AdmissionPlugin interface {
	// CheckAdmission enqueues an admission check to be performed by the plugin.
	//
	// The plugin will be asked if it's OK to deploy a package with the given pin
	// hosted on the CIPD service used by the running CIPD client.
	//
	// Returns a promise which is resolved when the result is available. If such
	// check is already pending (or has been done before), returns an existing
	// (perhaps already resolved) promise.
	CheckAdmission(pin common.Pin) Promise

	// ClearCache drops all resolved promises to free up some memory.
	ClearCache()

	// Close terminates the plugin (if it was running) and aborts all pending
	// checks.
	//
	// Tries to gracefully terminate the plugin, killing it with SIGKILL on the
	// context timeout or after 5 sec.
	//
	// Note that calling Close is not necessary if the plugin host itself
	// terminates. The plugin subprocess will be terminated by the host in this
	// case.
	Close(ctx context.Context)

	// Executable is a path to this plugin's executable.
	Executable() string
}

// Promise can be used to wait for a status of a check.
type Promise interface {
	// Wait blocks until the promise is fulfilled or the context expires.
	Wait(ctx context.Context) error
}
