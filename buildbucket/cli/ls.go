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
	"os/signal"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/cli"

	pb "go.chromium.org/luci/buildbucket/proto"
)

func cmdLS(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `ls [flags] [PATH [PATH...]]`,
		ShortDesc: "lists builds",
		LongDesc: doc(`
			Lists builds.

			Listed builds are sorted by creation time, descending.

			A PATH argument can be one of
			 - "<project>"
			 - "<project>/<bucket>"
			 - "<project>/<bucket>/<builder>"

			Multiple PATHs are connected with logical OR.
			Printed builds are deduplicated.
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &lsRun{}
			r.RegisterDefaultFlags(p)
			r.RegisterIDFlag()
			r.RegisterFieldFlags()
			r.clsFlag.Register(&r.Flags, doc(`
				CL URLs that builds must be associated with.

				Example: list builds of CL 1539021.
					bb ls -cl https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1539021/1
			`))
			r.tagsFlag.Register(&r.Flags, doc(`
				Tags that builds must have. Can be specified multiple times.
				All tags must be present.

				Example: list builds with tags "a:1" and "b:2".
					bb ls -t a:1 -t b:2
			`))
			r.Flags.BoolVar(&r.includeExperimental, "exp", false, doc(`
				Print experimental builds too
			`))
			r.Flags.Var(StatusFlag(&r.status), "status",
				fmt.Sprintf("Build status. Valid values: %s.", strings.Join(StatusFlagValues, ", ")))
			r.Flags.IntVar(&r.limit, "n", 0, doc(`
				Limit the number of builds to print. If 0, then unlimited.
				Can be passed as "-<number>", e.g. "ls -10".
			`))
			return r
		},
	}
}

type lsRun struct {
	printRun
	clsFlag
	tagsFlag

	status              pb.Status
	includeExperimental bool
	limit               int
}

func (r *lsRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	if err := r.initClients(ctx); err != nil {
		return r.done(ctx, err)
	}

	if r.limit < 0 {
		return r.done(ctx, fmt.Errorf("-n value must be non-negative"))
	}

	reqs, err := r.parseSearchRequests(ctx, args)
	if err != nil {
		return r.done(ctx, err)
	}

	searchCtx, cancelSearch := context.WithCancel(ctx)
	buildC := make(chan *pb.Build)
	searchErrC := make(chan error)
	go func() {
		err := protoutil.Search(searchCtx, buildC, r.client, reqs...)
		close(buildC)
		searchErrC <- err
	}()

	// Create a pager. Exit when it exits.
	var printDest io.Writer = os.Stdout
	less, err := r.startLess()
	// exitCodeForLess contains an exit code corresponding to the less outcome
	// when it exits.
	var exitCodeForLess chan int
	switch {
	case err == errLessUnavailable:
	case err != nil:
		return r.done(ctx, err)
	default:
		printDest = less

		// Swallow interrupts. Less is supposed to be quit by pressing q.
		// In particular, it does not respond to Ctrl+C.
		signal.Notify(make(chan os.Signal), os.Interrupt, os.Kill)

		exitCodeForLess = make(chan int)
		lessDone := make(chan struct{})
		go func() {
			exitCode, err := less.Wait()
			close(lessDone)
			// Stop searching if the user quit less.
			// This will cause buildC to close eventually.
			cancelSearch()
			if err != nil {
				exitCodeForLess <- r.done(ctx, err)
			} else {
				exitCodeForLess <- exitCode
			}
		}()
		// Do not exit before less.
		defer func() { <-lessDone }()
	}

	printer := newPrinter(printDest, false, time.Now)
	count := 0
	for b := range buildC {
		if err := r.printBuild(printer, b, count == 0); err != nil {
			if less != nil && !less.Alive() {
				// Most likely the user quit less by pressing q.
				// Just exit with its exit code.
				return <-exitCodeForLess
			}
			return r.done(ctx, err)
		}

		count++
		if count == r.limit {
			// We've reached the user-specified limit.
			return 0
		}
	}

	if err := <-searchErrC; err != nil && err != context.Canceled {
		return r.done(ctx, err)
	}

	return 0
}

// parseSearchRequests converts flags and arguments to search requests.
func (r *lsRun) parseSearchRequests(ctx context.Context, args []string) ([]*pb.SearchBuildsRequest, error) {
	baseReq, err := r.parseBaseRequest(ctx)
	if err != nil {
		return nil, err
	}

	if len(args) == 0 {
		return []*pb.SearchBuildsRequest{baseReq}, nil
	}

	ret := make([]*pb.SearchBuildsRequest, len(args))
	for i, path := range args {
		ret[i] = proto.Clone(baseReq).(*pb.SearchBuildsRequest)
		var err error
		if ret[i].Predicate.Builder, err = r.parsePath(path); err != nil {
			return nil, fmt.Errorf("invalid path %q: %s", path, err)
		}
	}
	return ret, nil
}

// parseBaseRequest returns a base SearchBuildsRequest without builder filter.
func (r *lsRun) parseBaseRequest(ctx context.Context) (*pb.SearchBuildsRequest, error) {
	ret := &pb.SearchBuildsRequest{
		Predicate: &pb.BuildPredicate{
			Tags:                r.Tags(),
			Status:              r.status,
			IncludeExperimental: r.includeExperimental,
		},
		PageSize: 100,
	}

	if r.limit > 0 && r.limit < int(ret.PageSize) {
		ret.PageSize = int32(r.limit)
	}

	// Prepare a field mask.
	var err error
	ret.Fields, err = r.FieldMask()
	if err != nil {
		return nil, err
	}
	for i, p := range ret.Fields.Paths {
		ret.Fields.Paths[i] = "builds.*." + p
	}

	if ret.Predicate.GerritChanges, err = r.clsFlag.retrieveCLs(ctx, r.httpClient); err != nil {
		return nil, err
	}
	return ret, nil
}

func (r *lsRun) parsePath(path string) (*pb.BuilderID, error) {
	bid := &pb.BuilderID{}
	switch parts := strings.Split(path, "/"); len(parts) {
	case 3:
		bid.Builder = parts[2]
		fallthrough
	case 2:
		bid.Bucket = parts[1]
		fallthrough
	case 1:
		bid.Project = parts[0]
	default:
		return nil, fmt.Errorf("got %d components, want 1-3", len(parts))
	}
	return bid, nil
}
