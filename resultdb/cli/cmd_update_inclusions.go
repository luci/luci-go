// Copyright 2020 The LUCI Authors.
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

package cli

import (
	"context"

	"github.com/maruel/subcommands"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

func cmdUpdateInclusions(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `update-inclusions [flags]`,
		ShortDesc: "add/remove included invocations",
		LongDesc: text.Doc(`
			Update included invocations.

			This adds or removes included invocations to/from the invocation
			set on LUCI_CONTEXT.
		`),
		Advanced: true,
		CommandRun: func() subcommands.CommandRun {
			r := &updateInclusionsRun{}
			r.registerFlags(p)
			return r
		},
	}
}

type updateInclusionsRun struct {
	baseCommandRun

	addInvocationIDs    []string
	removeInvocationIDs []string

	addInvocationNames    []string
	removeInvocationNames []string
}

func (r *updateInclusionsRun) registerFlags(p Params) {
	r.RegisterGlobalFlags(p)
	r.Flags.Var(flag.CommaList(&r.addInvocationIDs), "add", text.Doc(`
		ID(s) of the invocation(s) to include, use a comma-separated list
		to add multiple invocations.

		E.g.:
		    -add build:1234567,build:3456789
	`))
	r.Flags.Var(flag.StringSlice(&r.removeInvocationIDs), "remove", text.Doc(`
		ID(s) of the invocation(s) to include, use a comma-separated list
		to remove multiple invocations.

		E.g.:
		    -remove build:1234567,build:3456789
	`))
}

func (r *updateInclusionsRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	if len(args) != 0 {
		return r.done(errors.Reason("unexpected command line arguments %q", args).Err())
	}

	var err error

	if r.addInvocationNames, err = r.convertInvocationIDs(r.addInvocationIDs); err != nil {
		return r.done(err)
	}

	if r.removeInvocationNames, err = r.convertInvocationIDs(r.removeInvocationIDs); err != nil {
		return r.done(err)
	}

	if err := r.initClients(ctx); err != nil {
		return r.done(err)
	}

	if err := r.validateCurrentInvocation(); err != nil {
		return r.done(err)
	}

	return r.done(r.updateInclusions(ctx))
}

// convertInvocationIDs converts slice of strings containing invocation ids
// into a slice of strings containing invocation names.
func (r *updateInclusionsRun) convertInvocationIDs(ids []string) ([]string, error) {
	ret := make([]string, 0, len(ids))
	for _, id := range ids {
		if err := pbutil.ValidateInvocationID(id); err != nil {
			return nil, errors.Annotate(err, "invocation id %q", id).Err()
		}
		ret = append(ret, pbutil.InvocationName(id))
	}
	return ret, nil
}

func (r *updateInclusionsRun) updateInclusions(ctx context.Context) error {
	req := &pb.UpdateIncludedInvocationsRequest{
		IncludingInvocation: r.resultdbCtx.CurrentInvocation.Name,
		AddInvocations:      r.addInvocationNames,
		RemoveInvocations:   r.removeInvocationNames,
	}
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"update-token", r.resultdbCtx.CurrentInvocation.UpdateToken))
	_, err := r.recorder.UpdateIncludedInvocations(ctx, req)
	return err
}
