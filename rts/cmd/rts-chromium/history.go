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
	"fmt"
	"time"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	luciflag "go.chromium.org/luci/common/flag"

	"go.chromium.org/luci/rts/presubmit/eval/history"
	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"
)

func cmdPresubmitHistory(authOpt *auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `presubmit-history`,
		ShortDesc: "fetch presubmit history",
		LongDesc: text.Doc(`
			Fetch presubmit history, suitable for RTS evaluation.
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &presubmitHistoryRun{authOpt: authOpt}
			r.Flags.StringVar(&r.out, "out", "", "Path to the output file")
			r.Flags.Var(luciflag.Date(&r.startTime), "from", "Fetch results starting from this date; format: 2020-01-02")
			r.Flags.Var(luciflag.Date(&r.endTime), "to", "Fetch results until this date; format: 2020-01-02")
			return r
		},
	}
}

type presubmitHistoryRun struct {
	baseCommandRun
	out       string
	startTime time.Time
	endTime   time.Time

	authenticator *auth.Authenticator
	authOpt       *auth.Options

	w *history.Writer
}

func (r *presubmitHistoryRun) validate() error {
	switch {
	case r.out == "":
		return errors.New("-out is required")
	case r.startTime.IsZero():
		return errors.New("-from is required")
	case r.endTime.IsZero():
		return errors.New("-to is required")
	case r.endTime.Before(r.startTime):
		return errors.New("the -to date must not be before the -from date")
	default:
		return nil
	}
}

func (r *presubmitHistoryRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	if len(args) != 0 {
		return r.done(errors.New("unexpected positional arguments"))
	}

	if err := r.validate(); err != nil {
		return r.done(err)
	}

	r.authenticator = auth.NewAuthenticator(ctx, auth.OptionalLogin, *r.authOpt)

	// Create the history file.
	var err error
	if r.w, err = history.CreateFile(r.out); err != nil {
		return r.done(errors.Annotate(err, "failed to create the output file").Err())
	}
	defer r.w.Close()

	// Fetch the rejections.
	fmt.Printf("fetching CQ rejections...\n")
	err = r.rejectedPatchSets(ctx, func(rej *evalpb.Rejection) error {
		fmt.Print(".")
		return r.w.Write(&evalpb.Record{
			Data: &evalpb.Record_Rejection{Rejection: rej},
		})
	})
	if err != nil {
		return r.done(err)
	}

	fmt.Println()
	return r.done(r.w.Close())
}
