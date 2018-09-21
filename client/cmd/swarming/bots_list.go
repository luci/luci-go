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

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"google.golang.org/api/googleapi"

	"github.com/kr/pretty"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag"
)

func cmdBotsList(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "bots-list <options>",
		ShortDesc: "returns a list of bots",
		LongDesc:  "Returns a list of bots matching the given options.",
		CommandRun: func() subcommands.CommandRun {
			r := &botsListRun{}
			r.Init(defaultAuthOpts)
			return r
		},
	}
}

type botsListRun struct {
	commonFlags
	outfile string
	mp      bool
	nomp    bool
	fields  []googleapi.Field
}

func (b *botsListRun) Init(defaultAuthOpts auth.Options) {
	b.commonFlags.Init(defaultAuthOpts)

	b.Flags.StringVar(&b.outfile, "json", "", "Path to output JSON results.")
	b.Flags.BoolVar(&b.mp, "mp", false, "Only fetch Machine Provider bots.")
	b.Flags.BoolVar(&b.nomp, "nomp", false, "Exclude Machine Provider bots.")
	b.Flags.Var(flag.FieldSlice(&b.fields), "field", "Fields to include in a partial response. May be repeated.")
}

func (b *botsListRun) Parse() error {
	if err := b.commonFlags.Parse(); err != nil {
		return err
	}
	if b.mp && b.nomp {
		return errors.Reason("at most one of -mp and -nomp must be specified").Err()
	}
	return nil
}

func (b *botsListRun) main(a subcommands.Application) error {
	client, err := b.createAuthClient()
	if err != nil {
		return err
	}
	s, err := swarming.New(client)
	if err != nil {
		return err
	}
	s.BasePath = b.commonFlags.serverURL + "/_ah/api/swarming/v1/"
	call := s.Bots.List()
	if b.mp {
		call.IsMp("TRUE")
	} else if b.nomp {
		call.IsMp("FALSE")
	}
	// If no fields are specified, all fields will be returned. If any fields are
	// specified, ensure the cursor is specified so we can get subsequent pages.
	if len(b.fields) > 0 {
		b.fields = append(b.fields, "cursor")
	}
	call.Fields(b.fields...)
	// Keep calling as long as there's a cursor indicating more bots to list.
	var bots []*swarming.SwarmingRpcsBotInfo
	for {
		result, err := call.Do()
		if err != nil {
			return err
		}
		bots = append(bots, result.Items...)
		if result.Cursor == "" {
			break
		}
		call.Cursor(result.Cursor)
	}
	if !b.defaultFlags.Quiet {
		pretty.Println(bots)
	}
	var f *os.File
	if b.outfile != "" {
		f, err = os.OpenFile(b.outfile, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		j, err := json.MarshalIndent(bots, "", " ")
		if err != nil {
			return err
		}
		_, err = f.Write(j)
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *botsListRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	if err := b.Parse(); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	cl, err := b.defaultFlags.StartTracing()
	if err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	defer cl.Close()
	if err := b.main(a); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}
