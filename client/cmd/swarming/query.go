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

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/signals"
)

func cmdQuery(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "query <options> [method name]",
		ShortDesc: "Returns raw JSON information via an URL endpoint.",
		LongDesc: `Returns raw JSON information via an URL endpoint.

Examples:
  Raw task request and results:
    swarming query -S server-url.com task/123456/request
    swarming query -S server-url.com task/123456/result

  Listing all bots:
    swarming query -S server-url.com bots/list

  Listing last 10 tasks on a specific bot named 'bot1':
    swarming query -S server-url.com --limit 10 bot/bot1/tasks

  Listing last 10 tasks with tags os:Ubuntu-14.04 and pool:Chrome. Note that
  quoting is important!:
    swarming query -S server-url.com --limit 10 \
        'tasks/list?tags=os:Ubuntu-14.04&tags=pool:Chrome'
`,
		CommandRun: func() subcommands.CommandRun {
			r := &queryRun{}
			r.Init(defaultAuthOpts)
			return r
		},
	}
}

type queryRun struct {
	commonFlags
	outfile string
	limit   int
}

func (q *queryRun) Init(defaultAuthOpts auth.Options) {
	q.commonFlags.Init(defaultAuthOpts)
	q.Flags.StringVar(&q.outfile, "json", "", "Path to output JSON results. Implies quiet.")
	q.Flags.IntVar(&q.limit, "limit", 200, "Limit to enforce on limitless items (like number of tasks).")
}

func (q *queryRun) Parse() error {
	if err := q.commonFlags.Parse(); err != nil {
		return err
	}

	if q.defaultFlags.Quiet && q.outfile == "" {
		return errors.Reason("specify -json when using -quiet").Err()
	}

	if q.limit < 1 {
		return errors.Reason("invalid -limit %d, must be positive", q.limit).Err()
	}
	if q.outfile != "" {
		q.defaultFlags.Quiet = true
	}
	return nil
}

func (q *queryRun) main(a subcommands.Application, args []string) error {
	ctx, cancel := context.WithCancel(q.defaultFlags.MakeLoggingContext(os.Stderr))
	signals.HandleInterrupt(cancel)
	client, err := q.createAuthClient(ctx)
	if err != nil {
		return err
	}

	queryURL := q.commonFlags.serverURL + "/_ah/api/swarming/v1/" + args[len(args)-1]
	if strings.Contains(queryURL, "?") {
		queryURL = queryURL + "&limit=" + strconv.Itoa(q.limit)
	} else {
		queryURL = queryURL + "?limit=" + strconv.Itoa(q.limit)
	}

	// TODO(tikuta): use cursor?

	resp, err := client.Get(queryURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var out io.Writer = a.GetOut()

	if q.outfile != "" {
		f, err := os.Create(q.outfile)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}

	if _, err := io.Copy(out, resp.Body); err != nil {
		return err
	}

	return nil
}

func (q *queryRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	if err := q.Parse(); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	cl, err := q.defaultFlags.StartTracing()
	if err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	defer cl.Close()
	if err := q.main(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}
