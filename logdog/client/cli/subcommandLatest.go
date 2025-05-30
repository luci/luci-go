// Copyright 2016 The LUCI Authors.
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
	"io"
	"os"
	"time"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	log "go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/coordinator"
	"go.chromium.org/luci/logdog/common/renderer"
	"go.chromium.org/luci/logdog/common/types"
)

type latestCommandRun struct {
	subcommands.CommandRunBase

	raw bool
}

func newLatestCommand() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "latest [options] stream",
		ShortDesc: "Write the latest full log record in a stream to STDOUT.",
		LongDesc: "Write the latest full log record in a stream to STDOUT. If the stream " +
			"doesn't have any log entries, will block until a log entry is available.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &latestCommandRun{}

			cmd.Flags.BoolVar(&cmd.raw, "raw", false,
				"Reproduce original log stream, instead of attempting to render for humans.")
			return cmd
		},
	}
}

func (cmd *latestCommandRun) Run(scApp subcommands.Application, args []string, _ subcommands.Env) int {
	a := scApp.(*application)

	// User-friendly: trim any leading or trailing slashes from the path.
	if len(args) != 1 {
		log.Errorf(a, "Exactly one argument, the stream path, must be supplied.")
		return 1
	}

	var addr *types.StreamAddr
	var err error
	if addr, err = types.ParseURL(args[0]); err != nil {
		// Not a log stream address.
		project, path, _, err := a.splitPath(args[0])
		if err != nil {
			log.WithError(err).Errorf(a, "Invalid path specifier.")
			return 1
		}

		addr = &types.StreamAddr{Project: project, Path: types.StreamPath(path)}
		if err := addr.Path.Validate(); err != nil {
			log.Fields{
				log.ErrorKey: err,
				"project":    addr.Project,
				"path":       addr.Path,
			}.Errorf(a, "Invalid command-line stream path.")
			return 1
		}
	}

	coord, err := a.coordinatorClient(addr.Host)
	if err != nil {
		errors.Log(a, errors.Fmt("failed to create Coordinator client: %w", err))
		return 1
	}

	stream := coord.Stream(addr.Project, addr.Path)

	tctx, _ := a.timeoutCtx(a)
	le, st, err := cmd.getTailEntry(tctx, stream)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"project":    addr.Project,
			"path":       addr.Path,
		}.Errorf(a, "Failed to load latest record.")

		if err == context.DeadlineExceeded {
			return 2
		}
		return 1
	}

	// Render the entry.
	r := renderer.Renderer{
		Source:         &renderer.StaticSource{le},
		Raw:            cmd.raw,
		DatagramWriter: getDatagramWriter(a, &st.Desc),
	}
	if _, err := io.Copy(os.Stdout, &r); err != nil {
		log.WithError(err).Errorf(a, "failed to write to output")
		return 1
	}

	return 0
}

func (cmd *latestCommandRun) getTailEntry(c context.Context, s *coordinator.Stream) (
	*logpb.LogEntry, *coordinator.LogStream, error) {

	// Loop until we either hard fail or succeed.
	var st coordinator.LogStream

	delayTimer := clock.NewTimer(c)
	defer delayTimer.Stop()
	for {
		ls, err := s.Tail(c, coordinator.Complete(), coordinator.WithState(&st))

		// TODO(iannucci,dnj): use retry module + transient tags instead
		delayTimer.Reset(5 * time.Second)
		switch {
		case err == nil:
			return ls, &st, nil

		case err == coordinator.ErrNoSuchStream, ls == nil:
			log.WithError(err).Warningf(c, "No log entries, sleeping and retry.")

			if ar := <-delayTimer.GetC(); ar.Incomplete() {
				// Timer stopped prematurely.
				return nil, nil, ar.Err
			}

		default:
			return nil, nil, err
		}
	}
}
