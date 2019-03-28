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
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/maruel/subcommands"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/prpc"

	"go.chromium.org/luci/logdog/client/coordinator"
	"go.chromium.org/luci/logdog/common/fetcher"
	"go.chromium.org/luci/logdog/common/types"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	logpb "go.chromium.org/luci/logdog/api/logpb"
)

func cmdLog(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `log [flags] <BUILD> <STEP> [<LOG>...]`,
		ShortDesc: "prints step logs",
		LongDesc: `Prints step logs.

Argument BUILD can be an int64 build id or a string
<project>/<bucket>/<builder>/<build_number>, e.g. chromium/ci/linux-rel/1

Argument STEP is a build step name, e.g. "bot_update".
Use | as parent-child separator, e.g. "parent|child".

Arguments LOG is one ore more log names of the STEP. They will be multiplexed
by time. Defaults to stdout and stderr.
`,
		CommandRun: func() subcommands.CommandRun {
			r := &logRun{}
			r.SetDefaultFlags(defaultAuthOpts)
			return r
		},
	}
}

type logRun struct {
	baseCommandRun
}

type logRunArgs struct {
	build    string
	getBuild *buildbucketpb.GetBuildRequest
	step     string
	logs     stringset.Set
}

func (r *logRun) Run(a subcommands.Application, rawArgs []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	args, err := r.parseArgs(rawArgs)
	if err != nil {
		return r.done(ctx, err)
	}

	logs, err := r.getStepLogs(ctx, args)
	if err != nil {
		return r.done(ctx, err)
	}
	if len(logs) == 0 {
		return 0
	}

	return r.done(ctx, r.printLogs(ctx, logs))
}

func (r *logRun) parseArgs(args []string) (*logRunArgs, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("usage: bb log <BUILD> <STEP> [LOG...]")
	}

	ret := &logRunArgs{
		build: args[0],
		step:  args[1],
		logs:  stringset.NewFromSlice(args[2:]...),
	}
	var err error
	ret.getBuild, err = protoutil.ParseGetBuildRequest(ret.build)
	return ret, err
}

// getStepLogs fetches step logs.
func (r *logRun) getStepLogs(ctx context.Context, args *logRunArgs) ([]*buildbucketpb.Step_Log, error) {
	step, err := r.getStep(ctx, args)
	if err != nil {
		return nil, err
	}

	byName := map[string]*buildbucketpb.Step_Log{}
	for _, l := range step.Logs {
		byName[l.Name] = l
	}

	if len(args.logs) == 0 {
		logs := make([]*buildbucketpb.Step_Log, 0, 2)
		if log, ok := byName["stdout"]; ok {
			logs = append(logs, log)
		}
		if log, ok := byName["stderr"]; ok {
			logs = append(logs, log)
		}
		return logs, nil
	}

	logs := make([]*buildbucketpb.Step_Log, 0, len(args.logs))
	for name := range args.logs {
		l, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("log %q is not found in step %q of build %q", name, args.step, args.build)
		}
		logs = append(logs, l)
	}
	return logs, nil

}

// getStep fetches a step.
func (r *logRun) getStep(ctx context.Context, args *logRunArgs) (*buildbucketpb.Step, error) {
	client, err := r.newClient(ctx)
	if err != nil {
		return nil, err
	}

	args.getBuild.Fields = &field_mask.FieldMask{Paths: []string{"steps"}}
	build, err := client.GetBuild(ctx, args.getBuild, prpc.ExpectedCode(codes.NotFound))
	switch code := status.Code(err); {
	case code == codes.NotFound:
		return nil, fmt.Errorf("build not found")
	case code != codes.OK:
		return nil, err
	}

	for _, s := range build.Steps {
		if s.Name == args.step {
			return s, nil
		}
	}
	return nil, fmt.Errorf("step %q not found in build %q", args.step, args.build)
}

// printLogs fetches and prints the logs to io.Stdout/io.Stderr.
// Entries from different logs are multiplexed by timestamps.
func (r *logRun) printLogs(ctx context.Context, logs []*buildbucketpb.Step_Log) error {
	// Parse Log URLs.
	addrs := make([]*types.StreamAddr, len(logs))
	for i, l := range logs {
		var err error
		addrs[i], err = types.ParseURL(l.Url)
		if err != nil {
			return fmt.Errorf("log %q has unsupported URL %q", l.Name, l.Url)
		}

		if addrs[0].Host != addrs[i].Host {
			// unrealistic
			return fmt.Errorf("multiple different logdog hosts are not supported")
		}
	}

	// Create a client.
	httpClient, err := r.createHTTPClient(ctx)
	if err != nil {
		return err
	}
	client := coordinator.NewClient(&prpc.Client{
		C:    httpClient,
		Host: addrs[0].Host,
	})

	// Fetch descriptors and create fetchers.
	fetchers := make([]*fetcher.Fetcher, len(addrs))
	chans := make([]logChan, len(addrs))
	err = parallel.FanOutIn(func(work chan<- func() error) {
		for i, addr := range addrs {
			i := i
			stream := client.Stream(addr.Project, addr.Path)
			fetchers[i] = stream.Fetcher(ctx, nil)
			work <- func() error {
				s, err := stream.State(ctx)
				if err != nil {
					return err
				}
				if s.Desc.StreamType != logpb.StreamType_TEXT {
					return fmt.Errorf("log %q is not a text stream; only text streams are currently supported", logs[i].Name)
				}
				chans[i].start, err = ptypes.Timestamp(s.Desc.Timestamp)
				return err
			}
		}
	})
	if err != nil {
		return err
	}

	// Start fetching.
	for i := range chans {
		i := i
		c := make(chan logAndErr, 256)
		chans[i].c = c
		go func() {
			defer close(c)
			for {
				log, err := fetchers[i].NextLogEntry()
				c <- logAndErr{log, err}
				if err != nil {
					break
				}
			}
		}()
	}

	return multiplexLogs(chans, func(logIndex int, log *logpb.LogEntry) {
		out := os.Stdout
		if logs[logIndex].Name == "stderr" {
			out = os.Stderr
		}
		for _, line := range log.GetText().GetLines() {
			fmt.Fprintf(out, "%s", line.Value)
			fmt.Fprintf(out, line.Delimiter)
		}
	})
}

type logChan struct {
	start time.Time      // base time for log entries
	c     chan logAndErr // channel of log entries
}

type logAndErr struct {
	log *logpb.LogEntry
	err error
}

// multiplexLogs receives from chans and calls f in the order of log
// timestamps.
func multiplexLogs(chans []logChan, f func(chanIndex int, log *logpb.LogEntry)) error {
	heads := make([]logAndErr, len(chans))
	for i := range heads {
		heads[i] = <-chans[i].c
	}
	for {
		// Choose the earliest one.
		earliest := -1
		earliestTime := time.Time{}
		for i, h := range heads {
			if h.err == io.EOF {
				continue
			}
			if h.err != nil {
				return h.err
			}

			curTime := chans[i].start
			if h.log.TimeOffset != nil {
				offset, err := ptypes.Duration(h.log.TimeOffset)
				if err != nil {
					return err
				}
				curTime = curTime.Add(offset)
			}
			if earliest == -1 || curTime.Before(earliestTime) {
				earliest = i
				earliestTime = curTime
			}
		}
		if earliest == -1 {
			// all finished
			return nil
		}

		// Call f and replace the entry with the freshest one from the same
		// channel.
		f(earliest, heads[earliest].log)
		heads[earliest] = <-chans[earliest].c
	}
}
