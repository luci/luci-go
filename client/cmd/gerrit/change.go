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
	"go.chromium.org/luci/common/retry/transient"
)

type apiCallInput struct {
	ChangeID   string `json:"change_id,omitempty"`
	ProjectID  string `json:"project_id,omitempty"`
	RevisionID string `json:"revision_id,omitempty"`
	JSONInput  any    `json:"input,omitempty"`
	QueryInput any    `json:"params,omitempty"`
}

type apiCall func(context.Context, *gerrit.Client, *apiCallInput) (any, error)

type changeRunOptions struct {
	// These booleans indicate whether a value is required in a subcommand's JSON
	// input.
	changeID   bool
	projectID  bool
	revisionID bool
	jsonInput  any
	queryInput any
}

type changeRun struct {
	commonFlags
	changeRunOptions
	inputLocation string
	input         apiCallInput
	apiFunc       apiCall
}

type failureOutput struct {
	Message   string `json:"message"`
	Transient bool   `json:"transient"`
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
		return errors.Fmt("failed to decode input: %w", err)
	}

	// Verify we have a change ID if the command requires one.
	if c.changeID && len(c.input.ChangeID) == 0 {
		return errors.New("change_id is required")
	}

	// Verify we have a project ID if the command requires one.
	if c.projectID && len(c.input.ProjectID) == 0 {
		return errors.New("project_id is required")
	}

	// Verify we have a revision ID if the command requires one.
	if c.revisionID && len(c.input.RevisionID) == 0 {
		return errors.New("revision_id is required")
	}
	return nil
}

func (c *changeRun) writeOutput(v any) error {
	out := os.Stdout
	var err error
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
		c.writeOutput(failureOutput{
			Message:   err.Error(),
			Transient: transient.Tag.In(err),
		})
		return err
	}
	return c.writeOutput(v)
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

func cmdCreateBranch(authOpts auth.Options) *subcommands.Command {
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (any, error) {
		bi := input.JSONInput.(*gerrit.BranchInput)
		return client.CreateBranch(ctx, input.ProjectID, bi)
	}
	return &subcommands.Command{
		UsageLine: "create-branch <options>",
		ShortDesc: "creates a branch",
		LongDesc: `Creates a branch.

Input should contain a project ID and a JSON payload, e.g.
{
	"project_id": <project-id>,
	"input": <JSON payload>
}

More information on creating branches may be found here:
https://gerrit-review.googlesource.com/Documentation/rest-api-projects.html#create-branch`,
		CommandRun: func() subcommands.CommandRun {
			return newChangeRun(authOpts, changeRunOptions{
				projectID: true,
				jsonInput: &gerrit.BranchInput{},
			}, runner)
		},
	}
}

func cmdChangeAbandon(authOpts auth.Options) *subcommands.Command {
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (any, error) {
		ai := input.JSONInput.(*gerrit.AbandonInput)
		return client.AbandonChange(ctx, input.ChangeID, ai)
	}
	return &subcommands.Command{
		UsageLine: "change-abandon <options>",
		ShortDesc: "abandons a change",
		LongDesc: `Abandons a change in Gerrit.

Input should contain a change ID and optionally a JSON payload, e.g.
{
  "change_id": <change-id>,
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
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (any, error) {
		ci := input.JSONInput.(*gerrit.ChangeInput)
		return client.CreateChange(ctx, ci)
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
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (any, error) {
		req := input.QueryInput.(*gerrit.ChangeQueryParams)
		changes, _, err := client.ChangeQuery(ctx, *req)
		return changes, err
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
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (any, error) {
		opts := input.QueryInput.(*gerrit.ChangeDetailsParams)
		return client.ChangeDetails(ctx, input.ChangeID, *opts)
	}
	return &subcommands.Command{
		UsageLine: "change-detail <options>",
		ShortDesc: "gets details about a single change with optional fields",
		LongDesc: `Gets details about a single change with optional fields.

Input should contain a change ID and optionally query parameters, e.g.
{
  "change_id": <change-id>,
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

func cmdListChangeComments(authOpts auth.Options) *subcommands.Command {
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (any, error) {
		result, err := client.ListChangeComments(ctx, input.ChangeID, input.RevisionID)
		if err != nil {
			return nil, err
		}
		return result, nil
	}
	return &subcommands.Command{
		UsageLine: "list-change-comments <options>",
		ShortDesc: "gets all comments on a single change",
		LongDesc: `Gets all comments on a single change.

Input should contain a change ID, e.g.
{
  "change_id": <change-id>,
}

For more information on change-id, see
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id`,
		CommandRun: func() subcommands.CommandRun {
			return newChangeRun(authOpts, changeRunOptions{
				changeID: true,
			}, runner)
		},
	}
}

func cmdListRobotComments(authOpts auth.Options) *subcommands.Command {
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (any, error) {
		result, err := client.ListRobotComments(ctx, input.ChangeID, input.RevisionID)
		if err != nil {
			return nil, err
		}
		return result, nil
	}
	return &subcommands.Command{
		UsageLine: "list-robot-comments <options>",
		ShortDesc: "gets all robot comments on a single change",
		LongDesc: `Gets all robot comments on a single change.

Input should contain a change ID, e.g.
{
  "change_id": <change-id>,
}

For more information on change-id, see
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id`,
		CommandRun: func() subcommands.CommandRun {
			return newChangeRun(authOpts, changeRunOptions{
				changeID: true,
			}, runner)
		},
	}
}

func cmdChangesSubmittedTogether(authOpts auth.Options) *subcommands.Command {
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (any, error) {
		opts := input.QueryInput.(*gerrit.ChangeDetailsParams)
		return client.ChangesSubmittedTogether(ctx, input.ChangeID, *opts)
	}
	return &subcommands.Command{
		UsageLine: "changes-submitted-together <options>",
		ShortDesc: "lists Gerrit changes which are submitted together when Submit is called for a change",
		LongDesc: `Lists Gerrit changes which are submitted together when Submit is called for a change.

Input should contain a change ID and optionally query parameters, e.g.
{
  "change_id": <change-id>,
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
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (any, error) {
		ri := input.JSONInput.(*gerrit.ReviewInput)
		return client.SetReview(ctx, input.ChangeID, input.RevisionID, ri)
	}
	return &subcommands.Command{
		UsageLine: "set-review <options>",
		ShortDesc: "sets the review on a revision of a change",
		LongDesc: `Sets the review on a revision of a change.

Input should contain a change ID, a revision ID, and a JSON payload, e.g.
{
  "change_id": <change-id>,
  "revision_id": <revision-id>,
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

func cmdGetMergeable(authOpts auth.Options) *subcommands.Command {
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (any, error) {
		return client.GetMergeable(ctx, input.ChangeID, input.RevisionID)
	}
	return &subcommands.Command{
		UsageLine: "get-mergeable <options>",
		ShortDesc: "Checks if this change and revision are mergeable",
		LongDesc: `Does the mergeability check on a change and revision.

Input should contain a change ID, a revision ID, and a JSON payload, e.g.
{
  "change_id": <change-id>,
  "revision_id": <revision-id>
}

For more information on change-id, see
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id

For more information on revision-id, see
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#revision-id

More information on "get mergeable" may be found here:
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#get-mergeable`,
		CommandRun: func() subcommands.CommandRun {
			return newChangeRun(authOpts, changeRunOptions{
				changeID:   true,
				revisionID: true,
			}, runner)
		},
	}
}

func cmdSubmit(authOpts auth.Options) *subcommands.Command {
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (any, error) {
		si := input.JSONInput.(*gerrit.SubmitInput)
		return client.Submit(ctx, input.ChangeID, si)
	}
	return &subcommands.Command{
		UsageLine: "submit <options>",
		ShortDesc: "submit a change",
		LongDesc: `Submit a change.

Input should contain a change ID, e.g.
{
  "change_id": <change-id>,
}

For more information on change-id, see
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id`,
		CommandRun: func() subcommands.CommandRun {
			return newChangeRun(authOpts, changeRunOptions{
				changeID:  true,
				jsonInput: &gerrit.SubmitInput{},
			}, runner)
		},
	}
}

func cmdRebase(authOpts auth.Options) *subcommands.Command {
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (any, error) {
		ri := input.JSONInput.(*gerrit.RebaseInput)
		return client.RebaseChange(ctx, input.ChangeID, ri)
	}
	return &subcommands.Command{
		UsageLine: "rebase <options>",
		ShortDesc: "rebases a change",
		LongDesc: `rebases a change.

Input should contain a change ID, and optionally a JSON payload, e.g.
{
  "change_id": <change-id>,
  "input": <JSON payload>
}

For more information on change-id, see
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id

More information on "rebase" may be found here:
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#rebase-change`,
		CommandRun: func() subcommands.CommandRun {
			return newChangeRun(authOpts, changeRunOptions{
				changeID:  true,
				jsonInput: &gerrit.RebaseInput{},
			}, runner)
		},
	}
}

func cmdRestore(authOpts auth.Options) *subcommands.Command {
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (any, error) {
		ri := input.JSONInput.(*gerrit.RestoreInput)
		return client.RestoreChange(ctx, input.ChangeID, ri)
	}
	return &subcommands.Command{
		UsageLine: "restore <options>",
		ShortDesc: "restores a change",
		LongDesc: `restores a change.

Input should contain a change ID, and optionally a JSON payload, e.g.
{
  "change_id": <change-id>,
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

func cmdAccountQuery(authOpts auth.Options) *subcommands.Command {
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (any, error) {
		req := input.QueryInput.(*gerrit.AccountQueryParams)
		changes, _, err := client.AccountQuery(ctx, *req)
		return changes, err
	}
	return &subcommands.Command{
		UsageLine: "account-query <options>",
		ShortDesc: "queries Gerrit for accounts",
		LongDesc: `Queries Gerrit for accounts.

Input should contain query options, e.g. {"params": <query parameters as JSON>}

For more information on valid query parameters, see
https://gerrit-review.googlesource.com/Documentation/user-search-accounts.html#_search_operators`,
		CommandRun: func() subcommands.CommandRun {
			return newChangeRun(authOpts, changeRunOptions{
				queryInput: &gerrit.AccountQueryParams{},
			}, runner)
		},
	}
}

func cmdWip(authOpts auth.Options) *subcommands.Command {
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (any, error) {
		ri := input.JSONInput.(*gerrit.WipInput)
		return client.Wip(ctx, input.ChangeID, ri)
	}
	return &subcommands.Command{
		UsageLine: "wip <options>",
		ShortDesc: "sets work-in-progress on a change",
		LongDesc: `sets work-in-progress on a change.

Input should contain a change ID, and optionally a JSON payload, e.g.
{
  "change_id": <change-id>,
  "input": <JSON payload>
}

For more information on change-id, see
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id

More information on "set-work-in-progress" may be found here:
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#set-work-in-progress`,
		CommandRun: func() subcommands.CommandRun {
			return newChangeRun(authOpts, changeRunOptions{
				changeID:  true,
				jsonInput: &gerrit.WipInput{},
			}, runner)
		},
	}
}

func cmdReady(authOpts auth.Options) *subcommands.Command {
	runner := func(ctx context.Context, client *gerrit.Client, input *apiCallInput) (any, error) {
		ri := input.JSONInput.(*gerrit.WipInput)
		return client.Ready(ctx, input.ChangeID, ri)
	}
	return &subcommands.Command{
		UsageLine: "ready <options>",
		ShortDesc: "sets ready-for-review on a change",
		LongDesc: `sets ready-for-review on a change.

Input should contain a change ID, and optionally a JSON payload, e.g.
{
  "change_id": <change-id>,
  "input": <JSON payload>
}

For more information on change-id, see
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-id

More information on "set-ready-for-review" may be found here:
https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#set-ready-for-review`,
		CommandRun: func() subcommands.CommandRun {
			return newChangeRun(authOpts, changeRunOptions{
				changeID:  true,
				jsonInput: &gerrit.WipInput{},
			}, runner)
		},
	}
}
