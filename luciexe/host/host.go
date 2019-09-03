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

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/authctx"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/hardcoded/chromeinfra"
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

	// ExeAuth describes the LUCI Auth environment to run the user code within.
	//
	// `Run` will manage the lifecycle of ExeAuth entirely.
	//
	// If nil, defaults to DefaultExeAuth.
	//
	// It's recommended to make a copy of DefaultExeAuth and then set `ID` and
	// `KnownGerritHosts` to reasonable values.
	ExeAuth *authctx.Context

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

// DefaultExeAuth is the default value used for Options.ExeAuth if it's
// unspecified.
var DefaultExeAuth = authctx.Context{
	ID: "luciexe/host",
	Options: chromeinfra.SetDefaultAuthOptions(auth.Options{
		Scopes: []string{
			"https://www.googleapis.com/auth/cloud-platform",
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/gerritcodereview",
			"https://www.googleapis.com/auth/firebase",
		},
	}),
	EnableGitAuth:      true,
	EnableGCEEmulation: true,
	EnableDockerAuth:   true,
	EnableFirebaseAuth: true,
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
