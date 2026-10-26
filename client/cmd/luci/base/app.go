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
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
)

// AgentSkillHint is printed in help output to point users and AI agents to the luci skill.
const AgentSkillHint = "Tip: Refer to the 'luci' skill (go/luci-skill) for detailed test investigation instructions and workflows."

// FormatTitleWithSkillHint appends AgentSkillHint to title if not already present.
func FormatTitleWithSkillHint(title string) string {
	if strings.Contains(title, AgentSkillHint) {
		return title
	}
	if title == "" {
		return AgentSkillHint
	}
	return title + "\n\n" + AgentSkillHint
}

// HelpCmd returns a help subcommand based on subcommands.CmdHelp that includes
// AgentSkillHint in command-specific help output and supports nested subcommands.
func HelpCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: subcommands.CmdHelp.UsageLine,
		ShortDesc: subcommands.CmdHelp.ShortDesc,
		LongDesc:  subcommands.CmdHelp.LongDesc,
		CommandRun: func() subcommands.CommandRun {
			return &skillHelpRun{delegate: subcommands.CmdHelp.CommandRun()}
		},
	}
}

type skillHelpRun struct {
	delegate subcommands.CommandRun
}

func (r *skillHelpRun) GetFlags() *flag.FlagSet {
	return r.delegate.GetFlags()
}

func (r *skillHelpRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if len(args) > 1 {
		if cmd := subcommands.FindNearestCommand(a, args[0]); cmd != nil && strings.Contains(cmd.UsageLine, "<subcommand>") {
			return cmd.CommandRun().Run(a, append([]string{"help"}, args[1:]...), env)
		}
	}
	code := r.delegate.Run(a, args, env)
	if code == 0 && len(args) == 1 {
		fmt.Fprintf(a.GetErr(), "\n%s\n", AgentSkillHint)
	}
	return code
}

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
// If the first argument is a nested subcommand application and args contains a
// help flag alongside parent-level flags (or an immediate help flag), it
// translates that into "help" for the child application.
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
			if HasHelpFlag(args[1:2]) {
				out := []string{cmdName, "help"}
				for _, arg := range args[2:] {
					if strings.HasPrefix(arg, "-") {
						break
					}
					out = append(out, arg)
				}
				return out
			}
			if strings.HasPrefix(args[1], "-") {
				return []string{cmdName, "help"}
			}
			break
		}
	}
	return args
}

type subcommandApp struct {
	*cli.Application
	out io.Writer
	err io.Writer
}

var _ cli.ContextModificator = (*subcommandApp)(nil)

func (s *subcommandApp) GetOut() io.Writer {
	if s.out != nil {
		return s.out
	}
	return s.Application.GetOut()
}

func (s *subcommandApp) GetErr() io.Writer {
	if s.err != nil {
		return s.err
	}
	return s.Application.GetErr()
}

// RunSubcommandApp executes a nested subcommands application with standard help handling.
func RunSubcommandApp(a subcommands.Application, name, title string, commands []*subcommands.Command, args []string) int {
	cmds := make([]*subcommands.Command, len(commands))
	for i, c := range commands {
		if c == subcommands.CmdHelp {
			cmds[i] = HelpCmd()
		} else {
			cmds[i] = c
		}
	}
	args = NormalizeAppArgs(cmds, args)
	app := &subcommandApp{
		Application: &cli.Application{
			Name:     name,
			Title:    FormatTitleWithSkillHint(title),
			Commands: cmds,
			Context: func(ctx context.Context) context.Context {
				if m, ok := a.(cli.ContextModificator); ok {
					return m.ModifyContext(ctx)
				}
				return ctx
			},
		},
	}
	if a != nil {
		app.out = a.GetOut()
		app.err = a.GetErr()
	}
	return subcommands.Run(app, args)
}
