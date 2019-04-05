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

package cli

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
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"

	pb "go.chromium.org/luci/buildbucket/proto"
)

func cmdCollect(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `collect [flags] <BUILD> [<BUILD>...]`,
		ShortDesc: "wait for builds to end",
		LongDesc: doc(`
			Wait for builds to end.

			Argument BUILD can be an int64 build id or a string
			<project>/<bucket>/<builder>/<build_number>, e.g. chromium/ci/linux-rel/1

			Optionally writes build details into an output file as JSON array of
			bulidbucket.v2.Build proto messages.
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &collectRun{}
			r.RegisterDefaultFlags(p)
			r.Flags.DurationVar(&r.intervalArg, "interval", time.Minute, doc(`
				duration to wait between requests
			`))
			r.Flags.StringVar(&r.outputArg, "json-output", "", doc(`
					path to the file into which build details are written;
					nothing is written if unspecified
			`))
			return r
		},
		Advanced: true,
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

// dedupInt64s dedups int64s.
func dedupInt64s(nums []int64) []int64 {
	seen := make(map[int64]struct{}, len(nums))
	res := make([]int64, 0, len(nums))
	for _, x := range nums {
		if _, ok := seen[x]; !ok {
			seen[x] = struct{}{}
			res = append(res, x)
		}
	}
	return res
}

func collectBuildDetails(ctx context.Context, client pb.BuildsClient, buildIDs []int64, sleep func()) (map[int64]*pb.Build, error) {
	buildIDs = dedupInt64s(buildIDs)
	ended := make(map[int64]*pb.Build, len(buildIDs))
	for {
		breq := &pb.BatchRequest{}
		for _, id := range buildIDs {
			if _, ok := ended[id]; ok {
				continue
			}

			getReq := &pb.GetBuildRequest{
				Id:     id,
				Fields: getRequestFieldMask,
			}
			breq.Requests = append(breq.Requests, &pb.BatchRequest_Request{
				Request: &pb.BatchRequest_Request_GetBuild{
					GetBuild: getReq,
				},
			})
		}

		logging.Infof(ctx, "checking build statuses...")
		bresp, err := client.Batch(ctx, breq)
		if err != nil {
			return nil, err
		}

		for _, resp := range bresp.Responses {
			error := resp.GetError()
			code := codes.Code(error.GetCode())
			build := resp.GetGetBuild()
			switch {
			case grpcutil.IsTransientCode(code):
				logging.Warningf(ctx, "transient %s error: %s", code, error.Message)

			case code != codes.OK:
				return nil, fmt.Errorf("RPC error %s: %s", code, error.Message)

			case build.Status&pb.Status_ENDED_MASK != 0:
				ended[build.Id] = build
				logging.Infof(ctx, "%d ended: %s", build.Id, build.Status)
			}
		}

		logging.Infof(ctx, "%d are running/pending", len(buildIDs)-len(ended))
		if len(buildIDs) == len(ended) {
			break
		}
		sleep()
	}

	return ended, nil
}

func writeBuildDetails(buildIDs []int64, buildDetails map[int64]*pb.Build, outputPath string) error {
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

	// TODO(nodir): switch from Batch to concurrent requests.
	// Delete baseCommandRun.retrieveBuildIDs.
	buildIDs, err := r.retrieveBuildIDs(ctx, args)
	if err != nil {
		return r.done(ctx, err)
	}

	buildDetails, err := collectBuildDetails(ctx, r.client, buildIDs, func() {
		logging.Infof(ctx, "waiting %s before trying again...", r.intervalArg)
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
