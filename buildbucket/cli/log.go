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
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/maruel/subcommands"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/prpc"
	logpb "go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/coordinator"
	"go.chromium.org/luci/logdog/common/fetcher"
	"go.chromium.org/luci/logdog/common/types"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

func cmdLog(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `log [flags] <BUILD> <STEP> [<LOG>...]`,
		ShortDesc: "prints step logs",
		LongDesc: doc(`
			Prints step logs.

			Argument BUILD can be an int64 build id or a string
			<project>/<bucket>/<builder>/<build_number>, e.g. chromium/ci/linux-rel/1

			Argument STEP is a build step name, e.g. "bot_update".
			Use | as parent-child separator, e.g. "parent|child".

			Arguments LOG is one or more log names of the STEP. They will be multiplexed
			by time. Defaults to stdout and stderr.
			Log "stderr" is printed to stderr.
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &logRun{}
			r.RegisterDefaultFlags(p)
			return r
		},
	}
}

type logRun struct {
	baseCommandRun

	// args

	build    string
	getBuild *pb.GetBuildRequest
	step     string
	logs     stringset.Set
}

func (r *logRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	if err := r.initClients(ctx, nil); err != nil {
		return r.done(ctx, err)
	}

	if err := r.parseArgs(args); err != nil {
		return r.done(ctx, err)
	}

	// Find the step.
	steps, err := r.getSteps(ctx)
	if err != nil {
		return r.done(ctx, err)
	}
	var step *pb.Step
	for _, s := range steps {
		if s.Name == r.step {
			step = s
			break
		}
	}
	if step == nil {
		fmt.Fprintf(os.Stderr, "step %q not found in build %q. Available steps:\n", r.step, r.build)
		for _, s := range steps {
			fmt.Fprintf(os.Stderr, "  %s\n", s.Name)
		}
		return 1
	}

	// Find the logs.
	logByName := map[string]*pb.Log{}
	for _, l := range step.Logs {
		logByName[l.Name] = l
	}
	var logs []*pb.Log
	if len(r.logs) == 0 {
		logs = r.defaultLogs(logByName)
	} else {
		logs = make([]*pb.Log, 0, len(r.logs))
		for name := range r.logs {
			l, ok := logByName[name]
			if !ok {
				fmt.Fprintf(os.Stderr, "log %q not found in step %q of build %q. Available logs:\n", name, r.step, r.build)
				for _, l := range step.Logs {
					fmt.Fprintf(os.Stderr, "  %s\n", l.Name)
				}
				return 1
			}
			logs = append(logs, l)
		}
	}

	return r.done(ctx, r.printLogs(ctx, logs))
}

func (r *logRun) parseArgs(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: bb log <BUILD> <STEP> [LOG...]")
	}

	r.build = args[0]
	r.step = args[1]
	r.logs = stringset.NewFromSlice(args[2:]...)
	var err error
	r.getBuild, err = protoutil.ParseGetBuildRequest(r.build)
	return err
}

func (r *logRun) defaultLogs(available map[string]*pb.Log) []*pb.Log {
	logs := make([]*pb.Log, 0, 2)
	if log, ok := available["stdout"]; ok {
		logs = append(logs, log)
	}
	if log, ok := available["stderr"]; ok {
		logs = append(logs, log)
	}
	return logs
}

// getSteps fetches steps of the build.
func (r *logRun) getSteps(ctx context.Context) ([]*pb.Step, error) {
	r.getBuild.Fields = &field_mask.FieldMask{Paths: []string{"steps"}}
	build, err := r.buildsClient.GetBuild(ctx, r.getBuild, prpc.ExpectedCode(codes.NotFound))
	switch status.Code(err) {
	case codes.OK:
		return build.Steps, nil
	case codes.NotFound:
		return nil, fmt.Errorf("build not found")
	default:
		return nil, err
	}
}

// printLogs fetches and prints the logs to io.Stdout/io.Stderr.
// Entries from different logs are multiplexed by timestamps.
func (r *logRun) printLogs(ctx context.Context, logs []*pb.Log) error {
	if len(logs) == 0 {
		return nil
	}

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
	client := coordinator.NewClient(&prpc.Client{
		C:    r.httpClient,
		Host: addrs[0].Host,
	})

	// Fetch descriptors and create fetchers.
	fetchers := make([]*fetcher.Fetcher, len(addrs))
	m := logMultiplexer{Chans: make([]logChan, len(addrs))}
	err := parallel.FanOutIn(func(work chan<- func() error) {
		for i, addr := range addrs {
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
				m.Chans[i].start = s.Desc.Timestamp.AsTime()
				return err
			}
		}
	})
	if err != nil {
		return err
	}

	// Start fetching.
	for i := range m.Chans {
		c := make(chan logAndErr, 256)
		m.Chans[i].c = c
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

	stdout, stderr := newStdioPrinters(r.noColor)
	for {
		chanIndex, entry, err := m.Next()
		out := stdout
		switch {
		case err == io.EOF:
			return nil
		case err != nil:
			return err
		case logs[chanIndex].Name == "stderr":
			out = stderr
		}

		for _, line := range entry.GetText().GetLines() {
			out.f("%s%s", line.Value, line.Delimiter)
		}
		if out.Err != nil {
			return out.Err
		}
	}
}

type logChan struct {
	start time.Time      // base time for log entries
	c     chan logAndErr // channel of log entries
}

type logAndErr struct {
	*logpb.LogEntry
	err error
}

// logMultiplexer receives from Chans and multiplexes log entries by timestamp.
type logMultiplexer struct {
	Chans []logChan

	// internal state

	heads []logAndErr
}

// Next returns the next log entry.
// Returns io.EOF if there is no next log entry.
func (m *logMultiplexer) Next() (chanIndex int, log *logpb.LogEntry, err error) {
	if m.heads == nil {
		// initialization
		m.heads = make([]logAndErr, len(m.Chans))
		for i := range m.heads {
			m.heads[i] = <-m.Chans[i].c
		}
	}

	// Choose the earliest one.
	// Note: using heap is overkill here.
	// Most of the time we deal with only two logs.
	earliest := -1
	earliestTime := time.Time{}
	for i, h := range m.heads {
		if h.err == io.EOF {
			continue
		}
		if h.err != nil {
			return 0, nil, h.err
		}

		curTime := m.Chans[i].start
		if h.TimeOffset != nil {
			if err := h.TimeOffset.CheckValid(); err != nil {
				return 0, nil, err
			}
			offset := h.TimeOffset.AsDuration()
			curTime = curTime.Add(offset)
		}

		isEarlier := earliest == -1 ||
			curTime.Before(earliestTime) ||
			(curTime.Equal(earliestTime) && h.PrefixIndex < m.heads[earliest].PrefixIndex)
		if isEarlier {
			earliest = i
			earliestTime = curTime
		}
	}
	if earliest == -1 {
		// all finished
		return 0, nil, io.EOF
	}

	// Call f and replace the entry with the freshest one from the same
	// channel.
	earliestEntry := m.heads[earliest].LogEntry
	m.heads[earliest] = <-m.Chans[earliest].c
	return earliest, earliestEntry, nil
}
