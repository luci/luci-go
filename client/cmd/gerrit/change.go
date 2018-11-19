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
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/errors"
)

type apiCallInput struct {
	ChangeID   string      `json:"change_id,omitempty"`
	RevisionID string      `json:"revision_id,omitempty"`
	JSONInput  interface{} `json:"input,omitempty"`
	QueryInput interface{} `json:"params,omitempty"`
}

type apiCall func(context.Context, *gerrit.Client, *apiCallInput) (interface{}, error)

type changeRunOptions struct {
	changeID   bool
	revisionID bool
	jsonInput  interface{}
	queryInput interface{}
}

type changeRun struct {
	commonFlags
	changeRunOptions
	inputLocation string
	input         apiCallInput
	apiFunc       apiCall
}

func newChangeRun(authOpts auth.Options, cmdOpts changeRunOptions, apiFunc apiCall) *changeRun {
	c := changeRun{
		changeRunOptions: cmdOpts,
		apiFunc:          apiFunc,
	}
	c.commonFlags.Init(authOpts)
	c.Flags.StringVar(&c.inputLocation, "input", "", "(required) Path to file containing json input for the request (use '-' for stdin).")
	return &c
}

func (c *changeRun) Parse(a subcommands.Application, args []string) error {
	if err := c.commonFlags.Parse(); err != nil {
		return err
	}
	if len(args) != 0 {
		return errors.New("position arguments not expected")
	}
	if c.host == "" {
		return errors.New("must specify a host")
	}
	if c.inputLocation == "" {
		return errors.New("must specify input")
	}

	// Copy inputs from options to json-decodable input.
	c.input.JSONInput = c.changeRunOptions.jsonInput
	c.input.QueryInput = c.changeRunOptions.queryInput

	// Load json from file and decode.
	input := os.Stdin
	if c.inputLocation != "-" {
		f, err := os.Open(c.inputLocation)
		if err != nil {
			return err
		}
		defer f.Close()
		input = f
	}
	if err := json.NewDecoder(input).Decode(&c.input); err != nil {
		return errors.Annotate(err, "failed to decode input").Err()
	}

	// Verify we have a change ID if the command requires one.
	if c.changeID && len(c.input.ChangeID) == 0 {
		return errors.New("change-id is required")
	}

	// Verify we have a revision ID if the command requires one.
	if c.revisionID && len(c.input.RevisionID) == 0 {
		return errors.New("revision-id is required")
	}
	return nil
}

func (c *changeRun) main(a subcommands.Application) error {
	// Create auth client and context.
	authCl, err := c.createAuthClient()
	if err != nil {
		return err
	}
	ctx := c.defaultFlags.MakeLoggingContext(os.Stderr)

	// Create gerrit client and make call.
	g, err := gerrit.NewClient(authCl, c.host)
	if err != nil {
		return err
	}
	v, err := c.apiFunc(ctx, g, &c.input)
	if err != nil {
		return err
	}

	// Write output.
	out := os.Stdout
	if c.jsonOutput != "-" {
		out, err = os.Create(c.jsonOutput)
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

func cmdChangeAbandon(authOpts auth.Options) *subcommands.Command {
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (interface{}, error) {
		ai, _ := input.JSONInput.(*gerrit.AbandonInput)
		change, err := client.AbandonChange(ctx, input.ChangeID, ai)
		if err != nil {
			return nil, err
		}
		return change, nil
	}
	return &subcommands.Command{
		UsageLine: "change-abandon <options>",
		ShortDesc: "abandons a change",
		LongDesc: `Abandons a change in Gerrit.

Input should contain a change ID and optionally a JSON payload, e.g.
{
  "change-id": <change-id>,
  "input": <JSON payload>
}

For more information on change-id, see
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id

More information on abandoning changes may be found here:
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#abandon-change`,
		CommandRun: func() subcommands.CommandRun {
			return newChangeRun(authOpts, changeRunOptions{
				changeID:  true,
				jsonInput: &gerrit.AbandonInput{},
			}, runner)
		},
	}
}

func cmdChangeCreate(authOpts auth.Options) *subcommands.Command {
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (interface{}, error) {
		ci, _ := input.JSONInput.(*gerrit.ChangeInput)
		change, err := client.CreateChange(ctx, ci)
		if err != nil {
			return nil, err
		}
		return change, nil
	}
	return &subcommands.Command{
		UsageLine: "change-create <options>",
		ShortDesc: "creates a new change",
		LongDesc: `Creates a new change in Gerrit.

Input should contain a JSON payload, e.g. {"input": <JSON payload>}.

For more information, see https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#create-change`,
		CommandRun: func() subcommands.CommandRun {
			return newChangeRun(authOpts, changeRunOptions{
				jsonInput: &gerrit.ChangeInput{},
			}, runner)
		},
	}
}

func cmdChangeQuery(authOpts auth.Options) *subcommands.Command {
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (interface{}, error) {
		req, _ := input.QueryInput.(*gerrit.ChangeQueryParams)
		changes, _, err := client.ChangeQuery(ctx, *req)
		if err != nil {
			return nil, err
		}
		return changes, nil
	}
	return &subcommands.Command{
		UsageLine: "change-query <options>",
		ShortDesc: "queries Gerrit for changes",
		LongDesc: `Queries Gerrit for changes.

Input should contain query options, e.g. {"params": <query parameters as JSON>}

For more information on valid query parameters, see
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#query-changes`,
		CommandRun: func() subcommands.CommandRun {
			return newChangeRun(authOpts, changeRunOptions{
				queryInput: &gerrit.ChangeQueryParams{},
			}, runner)
		},
	}
}

func cmdChangeDetail(authOpts auth.Options) *subcommands.Command {
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (interface{}, error) {
		opts, _ := input.QueryInput.(*gerrit.ChangeDetailsParams)
		change, err := client.ChangeDetails(ctx, input.ChangeID, *opts)
		if err != nil {
			return nil, err
		}
		return change, nil
	}
	return &subcommands.Command{
		UsageLine: "change-detail <options>",
		ShortDesc: "gets details about a single change with optional fields",
		LongDesc: `Gets details about a single change with optional fields.

Input should contain a change ID and optionally query parameters, e.g.
{
  "change-id": <change-id>,
  "params": <query parameters as JSON>
}

For more information on change-id, see
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id

For more information on valid query parameters, see
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#list-changes`,
		CommandRun: func() subcommands.CommandRun {
			return newChangeRun(authOpts, changeRunOptions{
				changeID:   true,
				queryInput: &gerrit.ChangeDetailsParams{},
			}, runner)
		},
	}
}

func cmdSetReview(authOpts auth.Options) *subcommands.Command {
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (interface{}, error) {
		ri, _ := input.JSONInput.(*gerrit.ReviewInput)
		result, err := client.SetReview(ctx, input.ChangeID, input.RevisionID, ri)
		if err != nil {
			return nil, err
		}
		return result, nil
	}
	return &subcommands.Command{
		UsageLine: "set-review <options>",
		ShortDesc: "sets the review on a revision of a change",
		LongDesc: `Sets the review on a revision of a change.

Input should contain a change ID, a revision ID, and a JSON payload, e.g.
{
  "change-id": <change-id>,
  "revision-id": <revision-id>,
  "input": <JSON payload>
}

For more information on change-id, see
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id

For more information on revision-id, see
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#revision-id

More information on "set review" may be found here:
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#set-review`,
		CommandRun: func() subcommands.CommandRun {
			return newChangeRun(authOpts, changeRunOptions{
				changeID:   true,
				revisionID: true,
				jsonInput:  &gerrit.ReviewInput{},
			}, runner)
		},
	}
}

func cmdRestore(authOpts auth.Options) *subcommands.Command {
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (interface{}, error) {
		ri, _ := input.JSONInput.(*gerrit.RestoreInput)
		result, err := client.RestoreChange(ctx, input.ChangeID, ri)
		if err != nil {
			return nil, err
		}
		return result, nil
	}
	return &subcommands.Command{
		UsageLine: "restore <options>",
		ShortDesc: "restores a change",
		LongDesc: `restores a change.

Input should contain a change ID, and optionally a JSON payload, e.g.
{
  "change-id": <change-id>,
  "input": <JSON payload>
}

For more information on change-id, see
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id

More information on "restore" may be found here:
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#restore-change`,
		CommandRun: func() subcommands.CommandRun {
			return newChangeRun(authOpts, changeRunOptions{
				changeID:  true,
				jsonInput: &gerrit.RestoreInput{},
			}, runner)
		},
	}
}
