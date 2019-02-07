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
	"io/ioutil"
	"os"

	"google.golang.org/api/googleapi"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/client/internal/common"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag"
)

func cmdBots(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "bots <options>",
		ShortDesc: "lists bots",
		LongDesc:  "List bots matching the given options.",
		CommandRun: func() subcommands.CommandRun {
			r := &botsRun{}
			r.Init(defaultAuthOpts)
			return r
		},
	}
}

type botsRun struct {
	commonFlags
	outfile string
	mp      bool
	nomp    bool
	fields  []googleapi.Field
}

func (b *botsRun) Init(defaultAuthOpts auth.Options) {
	b.commonFlags.Init(defaultAuthOpts)

	b.Flags.StringVar(&b.outfile, "json", "", "Path to output JSON results. Implies quiet.")
	b.Flags.BoolVar(&b.mp, "mp", false, "Only fetch Machine Provider bots.")
	b.Flags.BoolVar(&b.nomp, "nomp", false, "Exclude Machine Provider bots.")
	b.Flags.Var(flag.FieldSlice(&b.fields), "field", "Fields to include in a partial response. May be repeated.")
}

func (b *botsRun) Parse() error {
	if err := b.commonFlags.Parse(); err != nil {
		return err
	}
	if b.mp && b.nomp {
		return errors.Reason("at most one of -mp and -nomp must be specified").Err()
	}
	if b.defaultFlags.Quiet && b.outfile == "" {
		return errors.Reason("specify -json when using -quiet").Err()
	}
	if b.outfile != "" {
		b.defaultFlags.Quiet = true
	}
	return nil
}

func (b *botsRun) main(a subcommands.Application) error {
	ctx := common.CancelOnCtrlC(b.defaultFlags.MakeLoggingContext(os.Stderr))
	client, err := b.createAuthClient(ctx)
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
		j, err := json.MarshalIndent(bots, "", " ")
		if err != nil {
			return err
		}
		fmt.Printf("%s\n", j)
	}
	if b.outfile != "" {
		j, err := json.Marshal(bots)
		if err != nil {
			return err
		}
		if err := ioutil.WriteFile(b.outfile, j, 0644); err != nil {
			return err
		}
	}
	return nil
}

func (b *botsRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
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
