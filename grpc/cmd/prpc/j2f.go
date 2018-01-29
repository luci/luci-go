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
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/flagpb"
	"go.chromium.org/luci/common/auth"
)

const (
	cmdJ2FUsage = `j2f [flags]`
	cmdJ2FDesc  = "converts a message from JSON format to flagpb format."
)

func cmdJ2F(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: cmdJ2FUsage,
		ShortDesc: cmdJ2FDesc,
		LongDesc: `Converts a message from JSON format to flagpb format.

Example:

  $ echo '{"name": "Lucy"}' | prpc fmt j2f
  -name Lucy

See also f2j subcommand.`,
		CommandRun: func() subcommands.CommandRun {
			c := &j2fRun{}
			c.registerBaseFlags(defaultAuthOpts)
			return c
		},
	}
}

type j2fRun struct {
	cmdRun
}

func (r *j2fRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	if len(args) != 0 {
		return r.argErr(cmdJ2FDesc, cmdJ2FUsage, "")
	}

	return r.done(jsonToFlags())
}

// jsonToFlags reads JSON from stdin, parses it to a message and
// prints the message in flagpb format.
func jsonToFlags() error {
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(os.Stdin); err != nil {
		return err
	}

	var msg map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &msg); err != nil {
		return err
	}

	flags, err := flagpb.MarshalUntyped(msg)
	if err != nil {
		return err
	}
	for i := range flags {
		flags[i] = escapeFlag(flags[i])
	}
	fmt.Println(strings.Join(flags, " "))
	return nil
}

// Escape flags

var (
	quotable               = ` "`
	toEscapeExceptQuotable = "\t\n\r'`"
	toEscape               = toEscapeExceptQuotable + quotable
	escapeReplacer         *strings.Replacer
	escapeReplacerInit     sync.Once
)

func initEscapeReplacer() {
	replacerArgs := make([]string, 0, len(toEscape)*2)
	for _, r := range toEscape {
		replacerArgs = append(replacerArgs, string(r), `\`+string(r))
	}
	escapeReplacer = strings.NewReplacer(replacerArgs...)
}

func escapeFlag(s string) string {
	if strings.ContainsAny(s, toEscapeExceptQuotable) {
		escapeReplacerInit.Do(initEscapeReplacer)
		return escapeReplacer.Replace(s)
	}
	if strings.ContainsAny(s, quotable) {
		return "'" + s + "'"
	}
	return s
}
