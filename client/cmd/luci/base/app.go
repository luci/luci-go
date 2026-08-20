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
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
)

// HasHelpFlag reports whether args contains -h, --help, or -help.
func HasHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			return true
		}
	}
	return false
}

// NormalizeAppArgs prepares arguments for a subcommand application.
// If args is empty or starts with a help flag, it returns []string{"help"}.
// If the first argument is a leaf command (not a nested subcommand application)
// and args contains a help flag anywhere, it translates the invocation into
// []string{"help", cmdName} so that the leaf command's help text and flags are
// displayed with exit code 0 regardless of extra positional arguments or flags.
func NormalizeAppArgs(commands []*subcommands.Command, args []string) []string {
	if len(args) == 0 {
		return []string{"help"}
	}
	if HasHelpFlag(args[:1]) {
		return []string{"help"}
	}
	if !HasHelpFlag(args[1:]) {
		return args
	}

	cmdName := args[0]
	for _, c := range commands {
		if c.Name() == cmdName {
			if !strings.Contains(c.UsageLine, "<subcommand>") {
				return []string{"help", cmdName}
			}
			break
		}
	}
	return args
}

// NormalizeHelpArgs translates top-level and subcommand -h/--help flags or empty arguments
// into the built-in "help" subcommand so that help text exits cleanly with code 0.
func NormalizeHelpArgs(args []string) []string {
	if len(args) == 0 {
		return []string{"help"}
	}
	if HasHelpFlag(args[:1]) {
		return []string{"help"}
	}
	return args
}

// RunSubcommandApp executes a nested subcommands application with standard help handling.
func RunSubcommandApp(a subcommands.Application, name, title string, commands []*subcommands.Command, args []string) int {
	args = NormalizeAppArgs(commands, args)
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
