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

package invocation

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/maruel/subcommands"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/client/cmd/luci/ids"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// Cmd returns the top-level command for `luci invocation`.
func Cmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "invocation <subcommand>",
		ShortDesc: "Inspect ResultDB root and legacy invocations",
		LongDesc: "Inspect ResultDB root and legacy invocations, including build descriptors\n" +
			"(branch, build target, build ID), extra builds, code sources, test definition\n" +
			"(system, name, run target, cluster), and execution/finalization state.",
		CommandRun: func() subcommands.CommandRun {
			return &invocationRun{af: af}
		},
	}
}

type invocationRun struct {
	subcommands.CommandRunBase
	af *base.AuthFlags
}

func (r *invocationRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return base.RunSubcommandApp(a, "luci invocation", "Invocation management", []*subcommands.Command{
		GetCmd(r.af),
		subcommands.CmdHelp,
	}, args)
}

// GetCmd returns the subcommand for `luci invocation get`.
func GetCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get [-invocationid <invocation_id>] [<target>]",
		ShortDesc: "Get invocation metadata, build, branch, target, sources, and state",
		LongDesc: "Get details of a root or legacy invocation, including primary build, extra builds,\n" +
			"tested code sources, definition properties, state, and verdict counts.\n\n" +
			"Examples:\n" +
			"  $ luci invocation get https://ci.chromium.org/ui/inv/ants-i14600010609614895\n" +
			"  $ luci invocation get -invocationid ants-i14600010609614895",
		CommandRun: func() subcommands.CommandRun {
			r := &invocationGetRun{af: af}
			if r.af != nil {
				r.af.Register(&r.Flags)
			}
			r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
			r.Flags.StringVar(&r.invocationID, "invocationid", "", "Invocation ID (e.g. ants-i... or build-867...)")
			r.Flags.BoolVar(&r.showMetadata, "show-metadata", false, "Show additional invocation tags, properties, and realm metadata")
			r.Flags.BoolVar(&r.jsonOut, "json", false, "Output invocation proto in JSON format")
			return r
		},
	}
}

type invocationGetRun struct {
	subcommands.CommandRunBase
	af           *base.AuthFlags
	client       pb.ResultDBClient
	out          io.Writer
	host         string
	invocationID string
	showMetadata bool
	jsonOut      bool
}

func (r *invocationGetRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if base.HasHelpFlag(args) {
		r.Flags.Usage()
		return 0
	}
	target, err := base.ParsePositionalTarget(&r.Flags, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	if target == "" && r.invocationID == "" {
		fmt.Fprintf(os.Stderr, "flag -invocationid (or a positional URL/ID) is required\n")
		return 1
	}

	ctx := cli.GetContext(a, r, env)
	ctx = format.WithDiscoveryCache(ctx)
	client := r.client
	if client == nil && r.af != nil {
		if err := r.af.Parse(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse auth flags: %s\n", err)
			return 1
		}
		var err error
		client, _, _, err = r.af.NewResultDBClient(ctx, r.host)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create resultdb client: %s\n", err)
			return 1
		}
	}

	extracted, err := ids.ExtractWithAuth(ctx, r.af, client, nil, target, ids.TargetBindings{
		InvocationID: &r.invocationID,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to extract IDs from %q: %s\n", target, err)
		return 1
	}
	if r.invocationID == "" {
		ids.PrintMissingFlagsError(os.Stderr, extracted, "flag -invocationid (or a positional URL/ID containing an invocation ID) is required")
		return 1
	}

	resolved, err := base.ResolveInvocation(ctx, client, r.invocationID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get invocation: %s\n", err)
		return 1
	}
	format.CacheResolvedInvocation(ctx, resolved)
	if extracted != nil {
		extracted.Legacy = resolved.Legacy
	}

	out := r.out
	if out == nil {
		out = os.Stdout
	}

	if r.jsonOut {
		ids.PrintDerivedIDs(os.Stderr, extracted)
		var msg proto.Message
		if resolved.Legacy {
			msg = resolved.Invocation
		} else {
			msg = resolved.RootInvocation
		}
		b, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(msg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to marshal JSON: %s\n", err)
			return 1
		}
		fmt.Fprintln(out, string(b))
		return 0
	}

	ids.PrintDerivedIDs(out, extracted)
	printInvocationDetails(ctx, out, client, resolved, r.showMetadata)
	return 0
}

func printInvocationDetails(ctx context.Context, out io.Writer, client pb.ResultDBClient, resolved *base.ResolvedInvocation, showMetadata bool) {
	if resolved == nil {
		return
	}
	if resolved.Legacy {
		inv := resolved.Invocation
		fmt.Fprintf(out, "Invocation:    invocations/%s (legacy)\n", resolved.ID)
		if inv == nil {
			return
		}
		if inv.GetState() != pb.Invocation_STATE_UNSPECIFIED {
			fmt.Fprintf(out, "Inv State:     %s\n", inv.GetState())
		}
		if s := format.FormatSources(inv.GetSourceSpec().GetSources()); s != "" {
			fmt.Fprintf(out, "Sources:       %s\n", s)
		}
		if inv.GetProducerResource() != "" {
			fmt.Fprintf(out, "Producer:      %s\n", inv.GetProducerResource())
		}
		if inv.GetCreateTime() != nil {
			fmt.Fprintf(out, "Created:       %s\n", inv.GetCreateTime().AsTime().UTC().Format(time.RFC3339))
		}
		if inv.GetFinalizeTime() != nil {
			fmt.Fprintf(out, "Finalized:     %s\n", inv.GetFinalizeTime().AsTime().UTC().Format(time.RFC3339))
		}
		if showMetadata {
			fmt.Fprintln(out)
			if inv.GetRealm() != "" {
				fmt.Fprintf(out, "Realm:        %s\n", inv.GetRealm())
			}
			if inv.GetBaselineId() != "" {
				fmt.Fprintf(out, "Baseline ID:  %s\n", inv.GetBaselineId())
			}
			if len(inv.GetTags()) > 0 {
				fmt.Fprintln(out, "Invocation Tags:")
				for _, tag := range inv.GetTags() {
					fmt.Fprintf(out, "  %s: %s\n", tag.GetKey(), tag.GetValue())
				}
			}
			if inv.GetProperties() != nil {
				if b, err := (protojson.MarshalOptions{Multiline: true, Indent: "  "}).Marshal(inv.GetProperties()); err == nil {
					fmt.Fprintf(out, "Invocation Properties:\n%s\n", string(b))
				}
			}
		}
		return
	}

	rootInv := resolved.RootInvocation
	rootInvName := "rootInvocations/" + resolved.ID
	var countsSuffix string
	if client != nil {
		if resAgg, err := client.QueryTestAggregations(ctx, &pb.QueryTestAggregationsRequest{
			Parent: rootInvName,
			Predicate: &pb.TestAggregationPredicate{
				AggregationLevel: pb.AggregationLevel_INVOCATION,
			},
			PageSize: 1,
		}); err == nil && resAgg != nil && len(resAgg.Aggregations) > 0 {
			if c := format.FormatVerdictCountsInline(resAgg.Aggregations[0].GetTotalVerdictCounts()); c != "" {
				countsSuffix = " " + c
			}
		}
	}
	fmt.Fprintf(out, "Invocation:    %s%s\n", rootInvName, countsSuffix)
	format.FprintRootInvocationMetadata(out, rootInv)
	if rootInv != nil {
		if rootInv.GetCreateTime() != nil {
			fmt.Fprintf(out, "Created:       %s\n", rootInv.GetCreateTime().AsTime().UTC().Format(time.RFC3339))
		}
		if rootInv.GetFinalizeTime() != nil {
			fmt.Fprintf(out, "Finalized:     %s\n", rootInv.GetFinalizeTime().AsTime().UTC().Format(time.RFC3339))
		}
		if summary := strings.TrimSpace(rootInv.GetSummaryMarkdown()); summary != "" {
			fmt.Fprintf(out, "Summary:\n  %s\n", strings.ReplaceAll(summary, "\n", "\n  "))
		}
		if showMetadata {
			fmt.Fprintln(out)
			format.FprintRootInvocationAdditionalMetadata(out, rootInv)
		}
	}
}
