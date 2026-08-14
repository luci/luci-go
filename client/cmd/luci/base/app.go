// Copyright 2026 The LUCI Authors.
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

package base

import (
	"context"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
)

// RunSubcommandApp executes a nested subcommands application with standard help handling.
func RunSubcommandApp(a subcommands.Application, name, title string, commands []*subcommands.Command, args []string) int {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "-h" || args[0] == "--help" || args[0] == "-help")) {
		args = []string{"help"}
	}
	app := &cli.Application{
		Name:     name,
		Title:    title,
		Commands: commands,
		Context: func(ctx context.Context) context.Context {
			if m, ok := a.(cli.ContextModificator); ok {
				return m.ModifyContext(ctx)
			}
			return ctx
		},
	}
	return subcommands.Run(app, args)
}
