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
	"fmt"
	"strings"

	"github.com/maruel/subcommands"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

func cmdUpdateInclusions(p Params) *subcommands.Command {
	return &subcommands.Command{
		// Prepend removals with "~".
		// Prefix "-" is not supported because of flag parsing.
		UsageLine: `update-inclusions [flags] [[~]INVOCATION_NAME_1] [[~]INVOCATION_NAME_2]...`,
		ShortDesc: "add/remove included invocations",
		LongDesc: text.Doc(`
			Update included invocations.

			This adds or removes included invocations to/from the current invocation.

			Invocation names prepended with '~' will me removed from the current
			invocation.
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
	addInvNames              []string
	removeInvNames           []string
	includingInvNameOverride string
	updateTokenOverride      string
}

func (r *updateInclusionsRun) registerFlags(p Params) {
	r.RegisterGlobalFlags(p)
	r.Flags.StringVar(&r.includingInvNameOverride, "including", "", text.Doc(`
		Name of the invocation to include/exclude others to/from.

		*NOTE: Intended for debugging/manual use only, it overrides the
		value given in LUCI_CONTEXT. For all other uses, LUCI_CONTEXT instead.
	`))
	r.Flags.StringVar(&r.updateTokenOverride, "update-token", "", text.Doc(`
		Token that allow updating the including invocation.

		*NOTE: Intended for debugging/manual use only, it overrides the
		value given in LUCI_CONTEXT.  It is a *security risk* to use tokens
		in command line arguments, prefer passing this via LUCI_CONTEXT instead.
	`))
}

func (r *updateInclusionsRun) parseArgs(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("No invocations to add or remove")
	}

	for _, arg := range args {
		name := arg
		remove := false
		if strings.HasPrefix(arg, "~") {
			name = strings.TrimPrefix(arg, "~")
			remove = true
		}
		if err := pbutil.ValidateInvocationName(name); err != nil {
			return errors.Annotate(err, "invocation name %q", name).Err()
		}
		if remove {
			r.removeInvNames = append(r.removeInvNames, name)
		} else {
			r.addInvNames = append(r.addInvNames, name)
		}
	}
	return nil
}

func (r *updateInclusionsRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	if err := r.parseArgs(args); err != nil {
		return r.done(err)
	}

	if err := r.initClients(ctx); err != nil {
		return r.done(err)
	}

	return r.done(r.updateInclusions(ctx))
}

func (r *updateInclusionsRun) updateInclusions(ctx context.Context) error {
	var currInv, token string

	if r.resultdbCtx != nil {
		currInv = r.resultdbCtx.CurrentInvocation.Name
		token = r.resultdbCtx.CurrentInvocation.UpdateToken
	}

	// Allow command line overrides.
	if r.includingInvNameOverride != "" {
		currInv = r.includingInvNameOverride
	}
	if r.updateTokenOverride != "" {
		token = r.updateTokenOverride
	}

	if currInv == "" {
		return errors.Annotate(fmt.Errorf(
			"including invocation name missing from both flags and LUCI_CONTEXT"), "").Err()
	}
	if token == "" {
		return errors.Annotate(fmt.Errorf(
			"invocation update token missing. Provide it either via flags or in LUCI_CONTEXT"), "").Err()
	}
	req := &pb.UpdateIncludedInvocationsRequest{
		IncludingInvocation: currInv,
		AddInvocations:      r.addInvNames,
		RemoveInvocations:   r.removeInvNames,
	}
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("update-token", token))
	_, err := r.recorder.UpdateIncludedInvocations(ctx, req)
	return err
}
