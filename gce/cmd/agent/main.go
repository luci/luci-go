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

// Package main contains an agent for connecting to a Swarming server.
package main

import (
	"bytes"
	"context"
	"os"
	"text/template"

	"cloud.google.com/go/compute/metadata"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/hardcoded/chromeinfra"

	"go.chromium.org/luci/gce/api/instances/v1"
	"go.chromium.org/luci/gce/vmtoken/client"
)

// substitute performs substitutions in a template string.
func substitute(c context.Context, s string, subs interface{}) (string, error) {
	t, err := template.New("tmpl").Parse(s)
	if err != nil {
		return "", err
	}
	buf := bytes.Buffer{}
	if err = t.Execute(&buf, subs); err != nil {
		return "", nil
	}
	return buf.String(), nil
}

// metaKey is the key to a *metadata.Client in the context.
var metaKey = "meta"

// withMetadata returns a new context with the given *metadata.Client installed.
func withMetadata(c context.Context, cli *metadata.Client) context.Context {
	return context.WithValue(c, &metaKey, cli)
}

// getMetadata returns the *metadata.Client installed in the current context.
func getMetadata(c context.Context) *metadata.Client {
	return c.Value(&metaKey).(*metadata.Client)
}

// newInstances returns a new instances.InstancesClient.
func newInstances(c context.Context, acc, host string) instances.InstancesClient {
	return instances.NewInstancesPRPCClient(&prpc.Client{
		C:    client.NewClient(getMetadata(c), acc),
		Host: host,
	})
}

// cmdRunBase is the base struct all subcommands should embed.
// Implements cli.ContextModificator.
type cmdRunBase struct {
	subcommands.CommandRunBase
	authFlags      authcli.Flags
	serviceAccount string
}

// Initialize registers common flags.
func (b *cmdRunBase) Initialize() {
	opts := chromeinfra.DefaultAuthOptions()
	b.authFlags.Register(b.GetFlags(), opts)
}

// ModifyContext returns a new context to be used by all commands. Implements
// cli.ContextModificator.
func (b *cmdRunBase) ModifyContext(c context.Context) context.Context {
	c = logging.SetLevel(gologger.StdConfig.Use(c), logging.Debug)
	opts, err := b.authFlags.Options()
	if err != nil {
		logging.Errorf(c, "%s", err.Error())
		panic("failed to get auth options")
	}
	b.serviceAccount = opts.GCEAccountName
	http, err := auth.NewAuthenticator(c, auth.OptionalLogin, opts).Client()
	if err != nil {
		logging.Errorf(c, "%s", err.Error())
		panic("failed to get authenticator")
	}
	meta := metadata.NewClient(http)
	swr := &SwarmingClient{
		Client:           http,
		PlatformStrategy: newStrategy(),
	}
	return withSwarming(withMetadata(c, meta), swr)
}

// New returns a new agent application.
func New() *cli.Application {
	return &cli.Application{
		Name:  "agent",
		Title: "GCE agent",
		Commands: []*subcommands.Command{
			subcommands.CmdHelp,
			newConnectCmd(),
		},
	}
}

func main() {
	os.Exit(subcommands.Run(New(), os.Args[1:]))
}
