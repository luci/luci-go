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

// Package host implements the 'Host Application' portion of the luciexe
// protocol.
//
// It manages a local Logdog Butler service, and also runs all LUCI Auth related
// daemons. It intercepts and interprets build.proto streams within the Butler
// context, merging them as necessary.
package host

import (
	"context"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	ldOutput "go.chromium.org/luci/logdog/client/butler/output"
)

// Options is an optional struct which allows you to control how Run operates.
type Options struct {
	// Where the butler will sink its data to.
	//
	// This is typicaly one of the implementations in
	// go.chromium.org/luci/logdog/client/butler/output.
	//
	// If nil, will use the 'null' logdog Output.
	LogdogOutput ldOutput.Output

	// Options for the hosted LUCI auth environment.
	//
	// Run will always use the "system" LUCI Auth account, if available, falling
	// back to the default account if not.
	Auth AuthOpts

	// The BaseDir becomes the root of this hosted luciexe session; All
	// directories (workdirs, tempdirs, etc.) are derived relative to this.
	//
	// If not provided, Run will pick a random directory under os.TempDir as the
	// value for BaseDir.
	//
	// The BaseDir (provided or picked) will be managed by Run; Prior to
	// execution, Run will ensure that the directory is empty.
	BaseDir string
}

// DefaultLUCIIdentityOrder is the default value of AuthOpts.LUCIIdentityOrder.
var DefaultLUCIIdentityOrder = []string{
	"system", "task", "",
}

// AuthOpts are options to control how host will set up the luci-auth
// environment.
type AuthOpts struct {
	// LUCIIdentityOrder is the order of LUCI Auth identities to try, in order.
	// Run will use the first identity found as the account for Git Auth, GCE
	// Emulation, etc.
	//
	// A value of "" means "use the default identity defined in LUCI_CONTEXT".
	//
	// If empty, defaults to DefaultLUCIIdentityOrder.
	LUCIIdentityOrder []string

	// These options allow you to enable the various auth services.
	//
	// If false, the given service won't be authenticated with luci-auth.
	EnableGitAuth      bool
	EnableGCEEmulation bool
	EnableDockerAuth   bool
	EnableFirebaseAuth bool

	// KnownGerritHosts is used with EnableGitAuth; it forces the given Gerrit
	// hosts to request authentication. This is useful for otherwise public Gerrit
	// hosts to ensure that the luciexe doesn't use the public/anonymous quota for
	// these repos.
	//
	// It is not necessary to list private Gerrit repos here.
	KnownGerritHosts []string
}

// Run executes `cb` in a "luciexe" host environment.
//
// The merged Build objects collected from the host environment will be pushed
// to the returned channel as it executes.
//
// The Context should be used for cancellation of the callback function; It's up
// to the function implementation to respect the cancelled context.
//
// When the callback function completes, Run closes the returned channel.
//
// Blocking the returned channel may block the execution of `cb`.
func Run(ctx context.Context, options *Options, cb func(context.Context) error) (<-chan *bbpb.Build, error) {
	// TODO(iannucci): implement
	return nil, nil
}
