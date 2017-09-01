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

// Command git-credential-luci is a Git credential helper.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/logging/gologger"
)

var cmdGet = &subcommands.Command{
	UsageLine: "get",
	ShortDesc: "get credential",
	LongDesc:  "Return a matching credential, if any exists.",
	CommandRun: func() subcommands.CommandRun {
		return &getRun{}
	},
}

type getRun struct {
	subcommands.CommandRunBase
}

func (g *getRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, g, env)
	auth := auth.NewAuthenticator(ctx, auth.SilentLogin, auth.Options{
		Method: auth.LUCIContextMethod,
		Scopes: []string{"https://www.googleapis.com/auth/gerritcodereview"},
	})
	t, err := auth.GetAccessToken(60 * time.Second)
	if err != nil {
		fmt.Errorf("cannot get access token: %v", err)
	}
	fmt.Printf("username=o\n")
	fmt.Printf("password=%s\n", t.AccessToken)
	return 0
}

var cmdStore = &subcommands.Command{
	UsageLine: "store",
	ShortDesc: "store credential",
	LongDesc:  "Store the credential, if applicable to the helper.",
	CommandRun: func() subcommands.CommandRun {
		return &ignoreRun{}
	},
}

var cmdErase = &subcommands.Command{
	UsageLine: "erase",
	ShortDesc: "erase credential",
	LongDesc:  "Remove a matching credential, if any, from the helper’s storage.",
	CommandRun: func() subcommands.CommandRun {
		return &ignoreRun{}
	},
}

type ignoreRun struct {
	subcommands.CommandRunBase
}

func (_ *ignoreRun) Run(_ subcommands.Application, _ []string, _ subcommands.Env) int {
	return 0
}

func main() {
	os.Exit(subcommands.Run(&cli.Application{
		Name:  "git-credential-luci",
		Title: "LUCI Git Credential Helper",
		Context: func(ctx context.Context) context.Context {
			return gologger.StdConfig.Use(ctx)
		},
		Commands: []*subcommands.Command{
			cmdGet,
			cmdStore,
			cmdErase,
		},
	}, nil))
}
