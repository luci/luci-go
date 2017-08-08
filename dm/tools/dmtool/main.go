// Copyright 2016 The LUCI Authors.
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

package main

import (
	"fmt"
	"os"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/logging/gologger"
)

// cmdRun is a base of all rpc subcommands.
type cmdRun struct {
	subcommands.CommandRunBase
	cmd *subcommands.Command
}

var logCfg = gologger.LoggerConfig{
	Format: `%{message}`,
	Out:    os.Stderr,
}

// argErr prints an err and usage to stderr and returns an exit code.
func (r *cmdRun) argErr(format string, a ...interface{}) int {
	if format != "" {
		fmt.Fprintf(os.Stderr, format+"\n", a...)
	}
	fmt.Fprintln(os.Stderr, r.cmd.ShortDesc)
	fmt.Fprintln(os.Stderr, r.cmd.UsageLine)
	fmt.Fprintln(os.Stderr, "\nFlags:")
	r.Flags.PrintDefaults()
	return -1
}

var application = &cli.Application{
	Name:  "dmtool",
	Title: "Dungeon Master CLI tool",
	Context: func(ctx context.Context) context.Context {
		return logCfg.Use(ctx)
	},
	Commands: []*subcommands.Command{
		cmdHashQuest,
		cmdVisQuery,
		subcommands.CmdHelp,
	},
}

func main() {
	os.Exit(subcommands.Run(application, os.Args[1:]))
}
