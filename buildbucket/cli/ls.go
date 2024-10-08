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

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/stringset"
	luciflag "go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/system/pager"

	bb "go.chromium.org/luci/buildbucket"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
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
				This flag is mutually exclusive with flag: -predicate.

				Example: list builds of CL 1539021.
					bb ls -cl https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1539021/1
			`))
			r.tagsFlag.Register(&r.Flags, doc(`
				Tags that builds must have. Can be specified multiple times.
				All tags must be present.
				This flag is mutually exclusive with flag: -predicate.

				Example: list builds with tags "a:1" and "b:2".
					bb ls -t a:1 -t b:2
			`))
			r.Flags.BoolVar(&r.includeExperimental, "exp", false, doc(`
				Print experimental builds too.
				This flag is mutually exclusive with flag: -predicate.
			`))
			r.experimentsFlag.Register(&r.Flags, doc(`
				Experiments that the builds must (or must not) have.
				This flag is mutually exclusive with flag: -predicate

				Can be specified multiple times. All provided values must match.

				Each value is in the form of `+"`[+-]experiment_name`"+`, where "+" indicates "must have"
				and "-" indicates "must not have".

				As a special case, "+luci.non_production" implies "-exp=true".

				Example: list builds with expirement luci.non_production:
				  bb ls -ex +luci.non_production

				Well-known experiments:
				  * `+bb.ExperimentNonProduction+`
				  * `+bb.ExperimentBackendAlt+`
					* `+bb.ExperimentBackendGo+`
				  * `+bb.ExperimentBBCanarySoftware+`
				  * `+bb.ExperimentBBAgent+`
					* `+bb.ExperimentBBAgentDownloadCipd+`
				  * `+bb.ExperimentBBAgentGetBuild+`
			`))
			r.Flags.Var(StatusFlag(&r.status), "status",
				fmt.Sprintf("Build status. Valid values: %s.\n"+
					"This flag is mutually exclusive with flag: -predicate.",
					strings.Join(statusFlagValuesName, ", ")))
			r.Flags.IntVar(&r.limit, "n", 0, doc(`
				Limit the number of builds to print. If 0, then unlimited.
				Can be passed as "-<number>", e.g. "ls -10".
			`))
			r.Flags.BoolVar(&r.noPager, "nopage", false, doc(`
				Disable paging.
			`))
			r.Flags.Var(luciflag.MessageSliceFlag(&r.predicates), "predicate", doc(`
				BuildPredicate that all builds in the response should match.

				Predicate is expressed in JSON format of BuildPredicate proto message:
					https://bit.ly/2RUjloG

				Multiple predicates are supported and connected with logical OR.
				This flag is mutally exclusive with -cl, -t, -exp, -status flags and
				any PATH arguments

				Example: list builds that is either with tag "a:1" or of builder "a/b/c"
					bb ls \
					-predicate '{"tags":[{"key":"a","value":"1"}]}' \
					-predicate '{"builder":{"project":"a","bucket":"b","builder":"c"}}'`))
			return r
		},
	}
}

// flagsMEWithPredicate defines a set of flags that are mutually exclusive with predicate flag
var flagsMEWithPredicate = stringset.NewFromSlice("cl", "exp", "status", "t")

type lsRun struct {
	printRun
	clsFlag
	tagsFlag
	experimentsFlag

	predicates []*pb.BuildPredicate
	status     pb.Status

	includeExperimental bool
	limit               int
	noPager             bool
}

func (r *lsRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	if err := r.initClients(ctx, nil); err != nil {
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
			err := protoutil.Search(ctx, buildC, r.buildsClient, reqs...)
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

	var pageSize int
	if pageSize = defaultPageSize; r.limit > 0 && r.limit < pageSize {
		pageSize = r.limit
	}

	fields, err := r.FieldMask()
	if err != nil {
		return nil, err
	}
	for i, p := range fields.Paths {
		fields.Paths[i] = "builds.*." + p
	}

	ret := make([]*pb.SearchBuildsRequest, len(predicates))
	for i, predicate := range predicates {
		ret[i] = &pb.SearchBuildsRequest{
			Predicate: predicate,
			PageSize:  int32(pageSize),
			// Creating defensive copy. Search method do mutate the field mask
			// append next_page_token
			Fields: proto.Clone(fields).(*field_mask.FieldMask),
		}
	}
	return ret, nil
}

// parseBuildPredicates converts flags and args to a slice of pb.BuildPredicate.
func (r *lsRun) parseBuildPredicates(ctx context.Context, args []string) ([]*pb.BuildPredicate, error) {
	if len(r.predicates) > 0 {
		// Get all flags that have been set with value provided on the command line.
		setFlags := stringset.New(r.Flags.NFlag())
		r.Flags.Visit(func(flag *flag.Flag) {
			setFlags.Add(flag.Name)
		})
		switch invalidFlags := setFlags.Intersect(flagsMEWithPredicate); {
		case invalidFlags.Len() > 0:
			return nil, fmt.Errorf("-predicate flag is mutually exclusive with flags: %q", invalidFlags.ToSortedSlice())
		case len(args) > 0:
			return nil, fmt.Errorf("-predicate flag is mutually exclusive with positional arguments")
		default:
			return r.predicates, nil
		}
	}

	basePredicate := &pb.BuildPredicate{
		Tags:                r.Tags(),
		Status:              r.status,
		IncludeExperimental: r.includeExperimental,
		Experiments:         r.experimentsFlag.experimentsFlat(),
	}

	if r.experimentsFlag.experiments[bb.ExperimentNonProduction] {
		basePredicate.IncludeExperimental = true
	}

	var err error
	if basePredicate.GerritChanges, err = r.clsFlag.retrieveCLs(ctx, r.httpClient, kRequirePatchset); err != nil {
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
