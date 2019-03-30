// Copyright 2019 The LUCI Authors.
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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/maruel/subcommands"
	"google.golang.org/genproto/protobuf/field_mask"

	"go.chromium.org/luci/auth"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/cli"
)

func cmdCollect(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `collect [flags] <BUILD> [<BUILD>...]`,
		ShortDesc: "wait for builds to end",
		LongDesc: `Wait for builds to end.

Argument BUILD can be an int64 build id or a string
<project>/<bucket>/<builder>/<build_number>, e.g. chromium/ci/linux-rel/1

Optionally writes build details into an output file as JSON array of
bulidbucket.v2.Build proto messages.`,
		CommandRun: func() subcommands.CommandRun {
			r := &collectRun{}
			r.RegisterGlobalFlags(defaultAuthOpts)
			r.Flags.DurationVar(
				&r.intervalArg, "interval", time.Minute,
				"duration to wait between requests")
			r.Flags.StringVar(&r.outputArg, "json-output", "", "path to the file "+
				"into which build details are written; nothing is written if "+
				"unspecified")
			return r
		},
	}
}

type collectRun struct {
	baseCommandRun
	intervalArg time.Duration
	outputArg   string
}

var getRequestFieldMask = &field_mask.FieldMask{
	Paths: []string{
		// Include all output properties, which are omitted by default.
		"output",

		// Since we add output props above, we must list default fields too.
		"builder",
		"create_time",
		"created_by",
		"end_time",
		"id",
		"input.experimental",
		"input.gerrit_changes",
		"input.gitiles_commit",
		"number",
		"start_time",
		"status",
		"update_time",
	},
}

func collectBuildDetails(ctx context.Context, client buildbucketpb.BuildsClient, buildIDs []int64, sleep func()) (map[int64]*buildbucketpb.Build, error) {
	unfinishedBuilds := buildIDs
	endedBuildDetails := make(map[int64]*buildbucketpb.Build, len(buildIDs))
	for {
		breq := &buildbucketpb.BatchRequest{}
		for _, buildID := range unfinishedBuilds {
			getReq := &buildbucketpb.GetBuildRequest{
				Id:     buildID,
				Fields: getRequestFieldMask,
			}
			breq.Requests = append(breq.Requests, &buildbucketpb.BatchRequest_Request{
				Request: &buildbucketpb.BatchRequest_Request_GetBuild{
					GetBuild: getReq,
				},
			})
		}

		fmt.Printf("Checking build statuses... ")
		bresp, err := client.Batch(ctx, breq)
		if err != nil {
			return nil, err
		}

		unfinishedBuilds = nil
		for _, resp := range bresp.Responses {
			build := resp.GetGetBuild()
			if build.Status&buildbucketpb.Status_ENDED_MASK != 0 {
				endedBuildDetails[build.Id] = build
			} else {
				unfinishedBuilds = append(unfinishedBuilds, build.Id)
			}
		}

		if len(unfinishedBuilds) == 0 {
			fmt.Printf("all ended\n")
			break
		}

		fmt.Printf(
			"%d still running/pending: %v\n", len(unfinishedBuilds), unfinishedBuilds)
		sleep()
	}

	return endedBuildDetails, nil
}

func writeBuildDetails(buildIDs []int64, buildDetails map[int64]*buildbucketpb.Build, outputPath string) error {
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer outputFile.Close()

	rawMessages := make([]json.RawMessage, len(buildIDs))
	m := &jsonpb.Marshaler{}
	buf := bytes.Buffer{}
	for i, buildID := range buildIDs {
		buf.Reset()
		if err := m.Marshal(&buf, buildDetails[buildID]); err != nil {
			return err
		}
		rawMessages[i] = append(json.RawMessage{}, buf.Bytes()...)
	}

	return json.NewEncoder(outputFile).Encode(rawMessages)
}

func (r *collectRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	if err := r.initClients(ctx); err != nil {
		return r.done(ctx, err)
	}

	buildIDs, err := r.retrieveBuildIDs(ctx, args)
	if err != nil {
		return r.done(ctx, err)
	}

	buildDetails, err := collectBuildDetails(ctx, r.client, buildIDs, func() {
		fmt.Printf("Waiting %s before trying again...\n", r.intervalArg)
		time.Sleep(r.intervalArg)
	})
	if err != nil {
		return r.done(ctx, err)
	}

	if r.outputArg != "" {
		if err := writeBuildDetails(buildIDs, buildDetails, r.outputArg); err != nil {
			return r.done(ctx, err)
		}
	}

	return r.done(ctx, nil)
}
