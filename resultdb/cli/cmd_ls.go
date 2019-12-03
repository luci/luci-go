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
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/pbutil"
)

func cmdLs(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `ls [flags] [INVOCATION_ID]...`,
		ShortDesc: "query results",
		LongDesc: text.Doc(`
			Query results.

			If no invocation ids are specified on the command line, read them from
			stdin separated by newline. Example:
			  bb chromium/ci/linux-rel -status failure -inv -10 | rdb ls
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &lsRun{}
			r.queryRun.registerFlags(p)
			// TODO(crbug.com/1021849): add flag -var
			// TODO(crbug.com/1021849): add flag -watch
			return r
		},
	}
}

type lsRun struct {
	queryRun
	invIDs []string
}

func (r *lsRun) parseArgs(args []string) error {
	r.invIDs = args
	if len(r.invIDs) == 0 {
		var err error
		if r.invIDs, err = readStdin(); err != nil {
			return err
		}
	}

	for _, id := range r.invIDs {
		if err := pbutil.ValidateInvocationID(id); err != nil {
			return errors.Annotate(err, "invocation id %q", id).Err()
		}
	}

	return r.queryRun.validate()
}

func (r *lsRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	if err := r.parseArgs(args); err != nil {
		return r.done(err)
	}

	if err := r.initClients(ctx); err != nil {
		return r.done(err)
	}

	return r.done(r.queryAndPrint(ctx, r.invIDs))
}

// readStdin reads all lines from os.Stdin.
func readStdin() ([]string, error) {
	// This context is used only to cancel the goroutine below.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-time.After(time.Second):
			fmt.Fprintln(os.Stderr, "expecting invocation ids on stdin...")
		case <-ctx.Done():
		}
	}()

	var ret []string
	stdin := bufio.NewReader(os.Stdin)
	for {
		line, err := stdin.ReadString('\n')
		if err == io.EOF {
			return ret, nil
		}
		if err != nil {
			return nil, err
		}
		ret = append(ret, strings.TrimSuffix(line, "\n"))
		cancel() // do not print the warning since we got something.
	}
}
