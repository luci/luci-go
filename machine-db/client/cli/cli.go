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

// Package cli contains the Machine Database command-line client.
package cli

import (
	"fmt"
	"os"

	"golang.org/x/net/context"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/authcli"
	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/grpc/prpc"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
)

// clientKey is the key to the context value withClient uses to store the RPC client.
var clientKey = "client"

// MultiValue allows a command-line option to be specified multiple times.
// Each instance is appended to a slice.
type MultiValue struct {
	values []string
}

// Set appends an instance of the command-line option.
func (m *MultiValue) Set(s string) error {
	m.values = append(m.values, s)
	return nil
}

// String returns a string representation of the current values.
func (m *MultiValue) String() string {
	return fmt.Sprintf("%s", m.values)
}

// Parameters contains parameters for constructing a new Machine Database command-line client.
type Parameters struct {
	// AuthOptions contains authentication-related options.
	AuthOptions auth.Options
	// Url is the URL of the Machine Database service to use.
	Url string
}

// createClient creates and returns a client which can make RPC requests to the Machine Database.
// Panics if the client cannot be created.
func createClient(c context.Context, params *Parameters) crimson.CrimsonClient {
	client, err := auth.NewAuthenticator(c, auth.InteractiveLogin, params.AuthOptions).Client()
	if err != nil {
		errors.Log(c, err)
		panic("failed to get authenticated HTTP client")
	}
	return crimson.NewCrimsonPRPCClient(&prpc.Client{
		C:    client,
		Host: params.Url,
	})
}

// getClient retrieves the client pointer embedded in the current context.
// The client pointer can be embedded in the current context using withClient.
func getClient(c context.Context) crimson.CrimsonClient {
	return c.Value(&clientKey).(crimson.CrimsonClient)
}

// withClient installs an RPC client pointer into the given context.
// It can be retrieved later on with getClient.
func withClient(c context.Context, client crimson.CrimsonClient) context.Context {
	return context.WithValue(c, &clientKey, client)
}

// GetApp returns the Machine Database command-line application.
func GetApp(params *Parameters) *cli.Application {
	return &cli.Application{
		Name:  "crimson",
		Title: "Machine Database client",
		Context: func(c context.Context) context.Context {
			cfg := gologger.LoggerConfig{
				Format: gologger.StdFormatWithColor,
				Out:    os.Stderr,
			}
			c = cfg.Use(c)
			c = withClient(c, createClient(c, params))
			return c
		},
		Commands: []*subcommands.Command{
			subcommands.CmdHelp,
			{}, // Create an empty command to separate groups of similar commands.

			// Authentication.
			authcli.SubcommandInfo(params.AuthOptions, "auth-info", true),
			authcli.SubcommandLogin(params.AuthOptions, "auth-login", false),
			authcli.SubcommandLogout(params.AuthOptions, "auth-logout", false),
			{},

			// Static entities.
			getDatacentersCmd(),
		},
	}
}

func Main(params *Parameters, args []string) int {
	return subcommands.Run(GetApp(params), os.Args[1:])
}
