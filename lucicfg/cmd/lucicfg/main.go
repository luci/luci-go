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

// Package main is the entry point for luci-cfg app.
package main

import (
	"os"

	"github.com/google/skylark/resolve"
	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"go.chromium.org/luci/client/versioncli"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/logging/gologger"
)

func init() {
	// Enable not-yet-standard features.
	resolve.AllowLambda = true
	resolve.AllowNestedDef = true
	resolve.AllowFloat = true
	resolve.AllowSet = true
}

// GetApplication returns luci-cfg application.
func GetApplication() subcommands.Application {
	return &cli.Application{
		Name:  "lucicfg",
		Title: "LUCI configuration files generator.",
		Context: func(ctx context.Context) context.Context {
			return gologger.StdConfig.Use(ctx)
		},
		Commands: []*subcommands.Command{
			cmdGenerate(),

			{},

			subcommands.CmdHelp,
			versioncli.CmdVersion("0.1"),
		},
	}
}

func main() {
	mathrand.SeedRandomly()
	os.Exit(subcommands.Run(GetApplication(), nil))
}
