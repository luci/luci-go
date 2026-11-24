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
	"os"
	"sort"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
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

// UnknownCommandHintFunc formats an optional actionable hint (without the
// leading "unknown command" line or trailing "Run '<app> help' for usage."
// line) when an unknown subcommand is invoked. Returning an empty string falls
// back to the default suggestion and available-subcommands hint.
type UnknownCommandHintFunc func(a subcommands.Application, unknownCmd string, rest []string) string

// HelpCmd returns a help subcommand based on subcommands.CmdHelp that includes
// AgentSkillHint in command-specific help output and supports nested subcommands.
func HelpCmd() *subcommands.Command {
	return HelpCmdWithHint(nil)
}

// HelpCmdWithHint returns a help subcommand like HelpCmd that prints an
// actionable hint via unknownHint (or the default command suggestions if nil or
// empty) when asked for help on an unknown command.
func HelpCmdWithHint(unknownHint UnknownCommandHintFunc) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: subcommands.CmdHelp.UsageLine,
		ShortDesc: subcommands.CmdHelp.ShortDesc,
		LongDesc:  subcommands.CmdHelp.LongDesc,
		CommandRun: func() subcommands.CommandRun {
			return &skillHelpRun{
				delegate:    subcommands.CmdHelp.CommandRun(),
				unknownHint: unknownHint,
			}
		},
	}
}

type skillHelpRun struct {
	delegate    subcommands.CommandRun
	unknownHint UnknownCommandHintFunc
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
	if len(args) >= 1 && subcommands.FindNearestCommand(a, args[0]) == nil {
		return printUnknownCommand(a, args[0], args[1:], r.unknownHint)
	}
	code := r.delegate.Run(a, args, env)
	if code == 0 && len(args) == 1 {
		fmt.Fprintf(a.GetErr(), "\n%s\n", AgentSkillHint)
	}
	return code
}

// RunApp normalizes help flags and executes a subcommands application, printing
// actionable hints when an unknown command is invoked.
func RunApp(a subcommands.Application, args []string, unknownHint UnknownCommandHintFunc) int {
	args = NormalizeAppArgs(a.GetCommands(), args)
	if len(args) > 0 {
		cmd := subcommands.FindNearestCommand(a, args[0])
		if cmd == nil {
			return printUnknownCommand(a, args[0], args[1:], unknownHint)
		}
		if len(args) > 1 && strings.HasPrefix(args[1], "-") && cmd.CommandRun != nil && strings.Contains(cmd.UsageLine, "<subcommand>") {
			return cmd.CommandRun().Run(a, args[1:], buildEnvMap(a))
		}
	}
	return subcommands.Run(a, args)
}

func buildEnvMap(a subcommands.Application) subcommands.Env {
	envVars := a.GetEnvVars()
	envMap := make(subcommands.Env, len(envVars))
	for k, v := range envVars {
		val, ok := os.LookupEnv(k)
		if !ok {
			val = v.Default
		}
		envMap[k] = subcommands.EnvVar{Value: val, Exists: ok}
	}
	return envMap
}

func printUnknownCommand(a subcommands.Application, unknownCmd string, rest []string, unknownHint UnknownCommandHintFunc) int {
	errOut := a.GetErr()
	fmt.Fprintf(errOut, "%s: unknown command %#q\n", a.GetName(), unknownCmd)

	var hint string
	if unknownHint != nil {
		hint = strings.TrimSpace(unknownHint(a, unknownCmd, rest))
	}
	if hint == "" {
		hint = defaultUnknownCommandHint(a, unknownCmd, rest)
	}
	if hint != "" {
		fmt.Fprintf(errOut, "\n%s\n", hint)
	}
	fmt.Fprintf(errOut, "\nRun '%s help' for usage.\n", a.GetName())
	return 2
}

func defaultUnknownCommandHint(a subcommands.Application, unknownCmd string, rest []string) string {
	cmds := a.GetCommands()
	avail := availableCommandNames(cmds)
	if len(avail) == 1 && looksLikeTargetArg(unknownCmd) {
		parts := make([]string, 0, 3+len(rest))
		parts = append(parts, a.GetName(), avail[0], ShellQuoteIfNeeded(unknownCmd))
		for _, arg := range rest {
			parts = append(parts, ShellQuoteIfNeeded(arg))
		}
		return fmt.Sprintf("Hint: '%s' only supports '%s'. Try:\n  %s", a.GetName(), avail[0], strings.Join(parts, " "))
	}

	var lines []string
	if suggestions := SuggestCommands(cmds, unknownCmd); len(suggestions) > 0 {
		lines = append(lines, fmt.Sprintf("Did you mean: %s?", strings.Join(suggestions, ", ")))
	}
	if len(avail) > 0 {
		lines = append(lines, fmt.Sprintf("Available subcommands for '%s': %s", a.GetName(), strings.Join(avail, ", ")))
	}
	return strings.Join(lines, "\n")
}

func availableCommandNames(commands []*subcommands.Command) []string {
	names := make([]string, 0, len(commands))
	for _, c := range commands {
		if c == nil {
			continue
		}
		name := c.Name()
		if name == "" || name == "help" {
			continue
		}
		names = append(names, name)
	}
	return names
}

// looksLikeTargetArg reports whether arg resembles a resource ID, URL, or flag
// rather than an English subcommand verb.
func looksLikeTargetArg(arg string) bool {
	if arg == "" {
		return false
	}
	if strings.HasPrefix(arg, "-") {
		return true
	}
	if strings.ContainsAny(arg, "/:._") {
		return true
	}
	for i := 0; i < len(arg); i++ {
		if arg[i] >= '0' && arg[i] <= '9' {
			return true
		}
	}
	return false
}

// SuggestCommands returns up to 3 closest command names from commands for input
// using edit distance and prefix/substring matching (excluding section headers
// and "help").
func SuggestCommands(commands []*subcommands.Command, input string) []string {
	in := strings.ToLower(strings.TrimSpace(input))
	if in == "" || looksLikeTargetArg(in) {
		return nil
	}

	type candidate struct {
		name string
		tier int // 1: distance 1 or prefix match; 2: distance 2 or component match
		dist int
	}

	var candidates []candidate
	bestTier := 3
	for _, name := range availableCommandNames(commands) {
		lower := strings.ToLower(name)
		d := levenshteinDistance(in, lower)
		tier := 0
		switch {
		case d == 1:
			tier = 1
		case strings.HasPrefix(lower, in):
			tier = 1
		case d == 2 && (len(in) >= 3 || (len(in) == 2 && in[0] == lower[0])):
			tier = 2
		case len(in) >= 3 && (strings.Contains(lower, in) || strings.HasPrefix(in, lower+"-")):
			tier = 2
		}
		if tier == 0 {
			continue
		}
		if tier < bestTier {
			bestTier = tier
		}
		candidates = append(candidates, candidate{name: name, tier: tier, dist: d})
	}
	if len(candidates) == 0 {
		return nil
	}

	filtered := candidates[:0]
	for _, c := range candidates {
		if c.tier == bestTier {
			filtered = append(filtered, c)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].dist != filtered[j].dist {
			return filtered[i].dist < filtered[j].dist
		}
		return filtered[i].name < filtered[j].name
	})

	const maxSuggestions = 3
	if len(filtered) > maxSuggestions {
		filtered = filtered[:maxSuggestions]
	}
	out := make([]string, len(filtered))
	for i, c := range filtered {
		out[i] = c.name
	}
	return out
}

func levenshteinDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	if len(rb) == 0 {
		return len(ra)
	}
	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(rb)]
}

// ParsePositionalTarget extracts at most one positional target argument from args
// and parses any leading or trailing flags into fs.
func ParsePositionalTarget(fs *flag.FlagSet, args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	if fs != nil && strings.HasPrefix(args[0], "-") && args[0] != "-" {
		if err := fs.Parse(args); err != nil {
			return "", err
		}
		args = fs.Args()
		if len(args) == 0 {
			return "", nil
		}
	}
	target := strings.TrimSpace(args[0])
	if len(args) > 1 {
		if fs == nil || !strings.HasPrefix(args[1], "-") || args[1] == "-" {
			return "", errors.New("unexpected positional arguments")
		}
		if err := fs.Parse(args[1:]); err != nil {
			return "", err
		}
		if len(fs.Args()) > 0 {
			return "", errors.New("unexpected positional arguments")
		}
	}
	return target, nil
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

// RunSubcommandApp executes a nested subcommands application with standard help
// and unknown-command handling.
func RunSubcommandApp(a subcommands.Application, name, title string, commands []*subcommands.Command, args []string) int {
	return RunSubcommandAppWithHint(a, name, title, commands, args, nil)
}

// RunSubcommandAppWithHint executes a nested subcommands application like
// RunSubcommandApp, using unknownHint to customize unknown-command guidance.
func RunSubcommandAppWithHint(a subcommands.Application, name, title string, commands []*subcommands.Command, args []string, unknownHint UnknownCommandHintFunc) int {
	cmds := make([]*subcommands.Command, len(commands))
	for i, c := range commands {
		if c == subcommands.CmdHelp {
			cmds[i] = HelpCmdWithHint(unknownHint)
		} else {
			cmds[i] = c
		}
	}
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
	return RunApp(app, args, unknownHint)
}
