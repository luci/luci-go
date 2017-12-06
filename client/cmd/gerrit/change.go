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

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/auth"
)

type apiCallInput struct {
	changeID string
	jsonInput string
	queryInput string
}

type apiCall func(context.Context, *gerrit.Client, *apiCallInput) (interface{}, error)

type changeRun struct {
	commonFlags
	changeID bool
	jsonInput bool
	queryInput bool
	input apiCallInput
	apiFunc apiCall
}

func newChangeRun(authOpts auth.Options, changeID, jsonInput bool, queryInput bool, apiFunc apiCall) *changeRun {
	c := changeRun{
		changeID: changeID,
		jsonInput: jsonInput,
		queryInput: queryInput,
		apiFunc: apiFunc,
	}
	c.commonFlags.Init(authOpts)
	if changeID {
		c.Flags.StringVar(&c.input.changeID, "change-id", "", "(required) an change ID of the form described above")
	}
	if jsonInput {
		c.Flags.StringVar(&c.input.jsonInput, "input", "", "(required) an input file containing the json input for the request")
	}
	if queryInput {
		c.Flags.StringVar(&c.input.queryInput, "params", "", "an input file containing query variables as json")
	}
	return &c
}

func (c *changeRun) Parse(a subcommands.Application, args []string) error {
	if err := c.commonFlags.Parse(); err != nil {
		return err
	}
	if len(args) != 0 {
		return errors.New("position arguments not expected")
	}
	if c.changeID && len(c.input.changeID) == 0 {
		return errors.New("change-id is required")
	}
	if c.jsonInput && len(c.input.jsonInput) == 0 {
		return errors.New("input is required")
	}
	return nil
}

func (c *changeRun) main(a subcommands.Application) error {
	authCl, err := c.createAuthClient()
	if err != nil {
		return err
	}
	ctx := c.defaultFlags.MakeLoggingContext(os.Stderr)

	g, err := gerrit.NewClient(authCl, c.host)
	if err != nil {
		return err
	}
	v, err := c.apiFunc(ctx, g, &c.input)
	if err != nil {
		return err
	}
	out := os.Stdout
	if c.jsonOutput != "-" {
		out, err := os.Create(c.jsonOutput)
		if err != nil {
			return err
		}
		defer out.Close()
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = out.Write(data)
	return err
}

func (c *changeRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	if err := c.Parse(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	if err := c.main(a); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}

func readInput(filename string, out interface{}) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	return json.NewDecoder(f).Decode(out)
}

func cmdChangeAbandon(authOpts auth.Options) *subcommands.Command {
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (interface{}, error) {
		var ai gerrit.AbandonInput
		if err := readInput(input.jsonInput, &ai); err != nil {
			return nil, err
		}
		change, err := client.AbandonChange(ctx, input.changeID, &ai)
		if err != nil {
			return nil, err
		}
		return change, nil
	}
	return &subcommands.Command{
		UsageLine: "change-abandon <options>",
		ShortDesc: "abandons a change",
		LongDesc: `Abandons a change in Gerrit.

The changeID parameter may be in any of the forms supported by Gerrit:
  - "4247"
  - "I8473b95934b5732ac55d26311a706c9c2bde9940"
  - etc. See the link below.
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id

More information on abandoning changes may be found here:
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#abandon-change`,
		CommandRun: func() subcommands.CommandRun {
			return newChangeRun(authOpts, true, true, false, runner)
		},
	}
}

func cmdChangeCreate(authOpts auth.Options) *subcommands.Command {
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (interface{}, error) {
		var ci gerrit.ChangeInput
		if err := readInput(input.jsonInput, &ci); err != nil {
			return nil, err
		}
		change, err := client.CreateChange(ctx, &ci)
		if err != nil {
			return nil, err
		}
		return change, nil
	}
	return &subcommands.Command{
		UsageLine: "change-create <options> url",
		ShortDesc: "creates a new change",
		LongDesc: `Creates a new change in Gerrit.
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#create-change`,
		CommandRun: func() subcommands.CommandRun {
			return newChangeRun(authOpts, false, true, false, runner)
		},
	}
}

func cmdChangeQuery(authOpts auth.Options) *subcommands.Command {
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (interface{}, error) {
		var req gerrit.ChangeQueryParams
		if err := readInput(input.queryInput, &req); err != nil {
			return nil, err
		}
		changes, _, err := client.ChangeQuery(ctx, req)
		if err != nil {
			return nil, err
		}
		return changes, nil
	}
	return &subcommands.Command{
		UsageLine: "change-query <options>",
		ShortDesc: "queries Gerrit for changes",
		LongDesc: `Queries Gerrit for changes.

The options flag is a list of strings, e.g. "CURRENT_REVISION" or "DETAILED_ACCOUNTS",
which tells Gerrit to return non-default properties for Change. The supported
strings for options are listed in Gerrit's api documentation at the link below:
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#list-changes

Example:

  gerrit change-query -options CURRENT_REVISION -options DETAILED_ACCOUNTS https://gerrit.host.example.com/repo owner:name@example.com`,
		CommandRun: func() subcommands.CommandRun {
			return newChangeRun(authOpts, false, false, true, runner)
		},
	}
}

func cmdChangeDetail(authOpts auth.Options) *subcommands.Command {
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (interface{}, error) {
		var opts gerrit.ChangeDetailsParams
		if err := readInput(input.queryInput, &opts); err != nil {
			return nil, err
		}
		change, err := client.ChangeDetails(ctx, input.changeID, opts)
		if err != nil {
			return nil, err
		}
		return change, nil
	}
	return &subcommands.Command{
		UsageLine: "change-detail <options> url id",
		ShortDesc: "gets details about a single change with optional fields",
		LongDesc: `Gets details about a single change with optional fields.

The changeID parameter may be in any of the forms supported by Gerrit:
  - "4247"
  - "I8473b95934b5732ac55d26311a706c9c2bde9940"
  - etc. See the link below.
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id

The options flag is a list of strings, e.g. "CURRENT_REVISION" or "DETAILED_ACCOUNTS",
which tells Gerrit to return non-default properties for Change. The supported
strings for options are listed in Gerrit's api documentation at the link below:
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#list-changes`,
		CommandRun: func() subcommands.CommandRun {
			return newChangeRun(authOpts, true, false, true, runner)
		},
	}
}
