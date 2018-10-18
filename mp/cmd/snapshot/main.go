// Copyright 2018 The LUCI Authors.
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

// Package main contains a tool which takes a snapshot of the given disk.
package main

import (
	"context"
	"os"

	"google.golang.org/api/compute/v1"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

// key is the context key used to retrieve the GCE API client.
var key = "service"

// getService retrieves the GCE API service embedded in the given context.
func getService(c context.Context) *compute.Service {
	return c.Value(&key).(*compute.Service)
}

// cmdRunBase is the base struct all subcommands should embed.
// Implements cli.ContextModificator.
type cmdRunBase struct {
	subcommands.CommandRunBase
	authFlags authcli.Flags
}

// Initialize registers common flags.
func (b *cmdRunBase) Initialize() {
	opts := chromeinfra.DefaultAuthOptions()
	opts.Scopes = []string{"https://www.googleapis.com/auth/compute"}
	b.authFlags.Register(b.GetFlags(), opts)
}

// ModifyContext returns a new context.
// Configures logging and embeds the GCE API service.
// Implements cli.ContextModificator.
func (b *cmdRunBase) ModifyContext(c context.Context) context.Context {
	c = logging.SetLevel(gologger.StdConfig.Use(c), logging.Debug)
	opts, err := b.authFlags.Options()
	if err != nil {
		logging.Errorf(c, "%s", err.Error())
		panic("failed to get auth options")
	}
	http, err := auth.NewAuthenticator(c, auth.OptionalLogin, opts).Client()
	if err != nil {
		logging.Errorf(c, "%s", err.Error())
		panic("failed to get authenticator")
	}
	srv, err := compute.New(http)
	if err != nil {
		logging.Errorf(c, "%s", err.Error())
		panic("failed to get GCE API service")
	}
	return context.WithValue(c, &key, srv)
}

// createService returns a new Compute Engine API service.
func createService(c context.Context, opts auth.Options) (*compute.Service, error) {
	http, err := auth.NewAuthenticator(c, auth.OptionalLogin, opts).Client()
	if err != nil {
		return nil, err
	}
	service, err := compute.New(http)
	if err != nil {
		return nil, err
	}
	return service, nil
}

// New returns a new snapshot application.
func New() *cli.Application {
	return &cli.Application{
		Name:  "snapshot",
		Title: "Machine Provider disk snapshot tool",
		Commands: []*subcommands.Command{
			subcommands.CmdHelp,
			getCreateSnapshotCmd(),
		},
	}
}

func main() {
	os.Exit(subcommands.Run(New(), os.Args[1:]))
}
