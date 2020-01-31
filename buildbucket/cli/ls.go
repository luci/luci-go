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
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"
	"google.golang.org/genproto/protobuf/field_mask"

	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/flag/protomsgsliceflag"
	"go.chromium.org/luci/common/system/pager"

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
				Can't be used with -predicate flag at the same time

				Example: list builds of CL 1539021.
					bb ls -cl https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1539021/1
			`))
			r.tagsFlag.Register(&r.Flags, doc(`
				Tags that builds must have. Can be specified multiple times.
				All tags must be present.
				Can't be used with -predicate flag at the same time.

				Example: list builds with tags "a:1" and "b:2".
					bb ls -t a:1 -t b:2
			`))
			r.Flags.BoolVar(&r.includeExperimental, "exp", false, doc(`
				Print experimental builds too.
				Can't be used with -predicate flag at the same time.
			`))
			r.Flags.Var(StatusFlag(&r.status), "status",
				fmt.Sprintf("Build status. Valid values: %s.\n"+
					"Can't be used with -predicate flag at the same time",
					strings.Join(statusFlagValuesName, ", ")))
			r.Flags.IntVar(&r.limit, "n", 0, doc(`
				Limit the number of builds to print. If 0, then unlimited.
				Can be passed as "-<number>", e.g. "ls -10".
			`))
			r.Flags.BoolVar(&r.noPager, "nopage", false, doc(`
				Disable paging.
			`))
			r.Flags.Var(protomsgsliceflag.MessageSliceFlag(&r.predicates),
				"predicate", doc(`
				BuildPredicate that all builds in the response should match.

				Predicate is expressed in JSON format with characters escaped.
				It will be deserialized to pb.BuildPredicate message:
					http://shortn/_wCltZ9IY7H

				Multiple predicates are supported and connected with logical OR.
				This flag can't be used with -cl, -t, -exp, -status flags and
				any PATH arguments at the sametime

				Example: list builds that is either with tag "a:1" or of builder
				"a/b/c"
					bb ls -predicate "{\"tags\":[{\"key\":\"a\",\"value\":\"1\"}]}" \
						-predicate "{\"builder\":{\"project\":\"a\",\"bucket\":\"b\",\"builder\":\"c\"}}"`))
			return r
		},
	}
}

type lsRun struct {
	printRun
	clsFlag
	tagsFlag

	predicates []*pb.BuildPredicate
	status     pb.Status

	includeExperimental bool
	limit               int
	noPager             bool
}

const defaultPageSize int32 = 100

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

	disableColors := r.noColor || shouldDisableColors()

	listBuilds := func(ctx context.Context, out io.WriteCloser) int {
		buildC := make(chan *pb.Build)
		errC := make(chan error)
		go func() {
			err := protoutil.Search(ctx, buildC, r.client, reqs...)
			close(buildC)
			errC <- err
		}()

		p := newPrinter(out, disableColors, time.Now)
		count := 0
		for b := range buildC {
			r.printBuild(p, b, count == 0)
			count++
			if count == r.limit {
				return 0
			}
		}

		if err := <-errC; err != nil && err != context.Canceled {
			return r.done(ctx, err)
		}
		return 0
	}

	if r.noPager {
		return listBuilds(ctx, os.Stdout)
	}
	return pager.Main(ctx, listBuilds)
}

// parseSearchRequests converts flags and arguments to search requests.
func (r *lsRun) parseSearchRequests(ctx context.Context, args []string) ([]*pb.SearchBuildsRequest, error) {
	predicates, err := r.parseBuildPredicates(ctx, args)
	if err != nil {
		return nil, err
	}

	ret := make([]*pb.SearchBuildsRequest, len(predicates))

	var pageSize int32
	if pageSize = defaultPageSize; r.limit > 0 && r.limit < int(pageSize) {
		pageSize = int32(r.limit)
	}

	var fields *field_mask.FieldMask
	if fields, err = r.FieldMask(); err != nil {
		return nil, err
	}
	for i, p := range fields.Paths {
		fields.Paths[i] = "builds.*." + p
	}

	for i, predicate := range predicates {
		ret[i] = &pb.SearchBuildsRequest{
			// TODO: See if defensive copy needed
			Predicate: predicate,
			PageSize:  pageSize,
			// Creating defensive copy. Search method do mutate the field mask
			// append next_page_token
			Fields: proto.Clone(fields).(*field_mask.FieldMask),
		}
	}
	return ret, nil
}

// parseBuildPredicates converts flags and args to slice of pb.BuildPredicate
func (r *lsRun) parseBuildPredicates(ctx context.Context, args []string) ([]*pb.BuildPredicate, error) {
	// Get all flags that have been set with value provided in command line
	setFlags := stringset.New(r.Flags.NFlag())
	r.Flags.Visit(func(flag *flag.Flag) {
		setFlags.Add(flag.Name)
	})

	// User supplied BuildPredicate message instance
	if setFlags.Has("predicate") {
		invalidFlags := setFlags.Intersect(stringset.NewFromSlice(
			"cl", "t", "exp", "status"))
		if invalidFlags.Len() > 0 {
			return nil, fmt.Errorf("-predicate flag is mutually exclusive with"+
				"-cl, -t, -exp, -status flags. In reality, Flags: %s are declared",
				strings.Join(invalidFlags.ToSlice(), ", "))
		} else if len(args) > 0 {
			return nil, fmt.Errorf("-predicate flag is mutually exclusive with"+
				" PATH arugment. Got PATH: %s", strings.Join(args, ", "))
		} else {
			return r.predicates, nil
		}
	}

	var err error
	basePredicate := &pb.BuildPredicate{
		Tags:                r.Tags(),
		Status:              r.status,
		IncludeExperimental: r.includeExperimental,
	}

	if basePredicate.GerritChanges, err = r.clsFlag.retrieveCLs(
		ctx, r.httpClient, kRequirePatchset); err != nil {
		return nil, err
	}

	if len(args) == 0 {
		return []*pb.BuildPredicate{basePredicate}, nil
	}

	// PATH arguments
	predicates := make([]*pb.BuildPredicate, len(args))
	for i, path := range args {
		predicates[i] = proto.Clone(basePredicate).(*pb.BuildPredicate)
		if predicates[i].Builder, err = r.parsePath(path); err != nil {
			return nil, fmt.Errorf("invalid path %q: %s", path, err)
		}
	}
	return predicates, nil
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
