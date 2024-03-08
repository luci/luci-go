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

// Binary main is a CAS client.
//
// This is a thin wrapper of remote-apis-sdks to upload/download files from CAS
// efficiently.
package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/client/casclient"
	"go.chromium.org/luci/client/cmd/cas/casimpl"
	"go.chromium.org/luci/client/versioncli"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

type authFlags struct {
	flags       authcli.Flags
	defaultOpts auth.Options
	parsedOpts  *auth.Options
}

func (af *authFlags) Register(f *flag.FlagSet) {
	af.flags.Register(f, af.defaultOpts)
}

func (af *authFlags) Parse() error {
	opts, err := af.flags.Options()
	if err != nil {
		return err
	}
	af.parsedOpts = &opts
	return nil
}

func (af *authFlags) NewRBEClient(ctx context.Context, addr string, instance string, readOnly bool) (*client.Client, error) {
	if af.parsedOpts == nil {
		return nil, errors.Reason("AuthFlags.Parse() must be called").Err()
	}
	return casclient.NewLegacy(ctx, addr, instance, *af.parsedOpts, readOnly)
}

func getApplication() *cli.Application {
	authOpts := chromeinfra.DefaultAuthOptions()
	af := &authFlags{defaultOpts: authOpts}

	return &cli.Application{
		Name:  "cas",
		Title: "Client tool to access CAS.",
		Context: func(ctx context.Context) context.Context {
			return gologger.StdConfig.Use(ctx)
		},
		Commands: []*subcommands.Command{
			subcommands.CmdHelp,

			casimpl.CmdArchive(af),
			casimpl.CmdDownload(af),

			authcli.SubcommandInfo(authOpts, "whoami", false),
			authcli.SubcommandLogin(authOpts, "login", false),
			authcli.SubcommandLogout(authOpts, "logout", false),

			versioncli.CmdVersion(casimpl.Version),
		},
	}
}

func main() {
	log.SetFlags(log.Lmicroseconds)
	app := getApplication()
	os.Exit(subcommands.Run(app, nil))
}
