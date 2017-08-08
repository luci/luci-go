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

package main

import (
	"flag"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/cli"
)

func cmdInconsistency(authOptions auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `inconsistency`,
		ShortDesc: "finds inconsistencies between buildbot and swarmbucket builders",
		LongDesc:  "Finds inconsistencies between buildbot and swarmbucket builders",
		Advanced:  true,
		CommandRun: func() subcommands.CommandRun {
			r := &inconsistencyRun{}
			r.SetDefaultFlags(authOptions)
			r.Flags.Int64Var(&r.since, "since", 0, "analyze builds since this timestamp. Defaults to 10 days ago.")
			r.Flags.Var(&r.builder1, "builder1", `colon-separated bucket and builder, e.g. "master.tryserver.chromium.linux:linux_chromium_rel_ng"`)
			r.Flags.Var(&r.builder2, "builder2", `colon-separated bucket and builder of the alternative builder to compare to"`)
			return r
		},
	}
}

type inconsistencyRun struct {
	baseCommandRun
	since              int64
	builder1, builder2 builderID
	client             *buildbucket.Service
}

type builderID struct {
	Bucket  string
	Builder string
}

func (b *builderID) Set(v string) error {
	parts := strings.SplitN(v, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("does not have ':'")
	}
	parsed := builderID{parts[0], parts[1]}
	if err := parsed.Validate(); err != nil {
		return err
	}
	*b = parsed
	return nil
}

func (b builderID) String() string {
	return b.Bucket + ":" + b.Builder
}

func (b *builderID) Validate() error {
	if b.Bucket == "" {
		return fmt.Errorf("bucket unspecified")
	}
	if b.Builder == "" {
		return fmt.Errorf("builder unspecified")
	}
	return nil
}

func (r *inconsistencyRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	if len(args) > 0 {
		return r.done(ctx, fmt.Errorf("unexpected arguments: %s", flag.Args()))
	}

	if err := r.builder1.Validate(); err != nil {
		return r.done(ctx, fmt.Errorf("invalid -builder1: %s", err))
	}
	if err := r.builder2.Validate(); err != nil {
		return r.done(ctx, fmt.Errorf("invalid -builder2: %s", err))
	}

	client, err := r.createClient(ctx)
	if err != nil {
		return r.done(ctx, err)
	}
	r.client, err = buildbucket.New(client.HTTP)
	if err != nil {
		return r.done(ctx, err)
	}
	r.client.BasePath = client.baseURL.String()

	var startingFrom time.Time
	var duration time.Duration
	if r.since == 0 {
		duration = 240 * time.Hour
		startingFrom = time.Now().Add(-duration)
	} else {
		startingFrom = time.Unix(r.since, 0)
		duration = time.Since(startingFrom)
	}

	if err := r.compareBuilder(ctx, startingFrom); err != nil {
		return r.done(ctx, err)
	}
	return 0
}

func (r *inconsistencyRun) compareBuilder(ctx context.Context, startingFrom time.Time) error {
	fmt.Printf("searching for all builds since timestamp %d till %d...\n",
		startingFrom.Unix(), time.Now().Unix())
	// We will actually fetch builds after after time.Now too, but it is fine.
	builds1, err := r.fetchBuilds(r.builder1, startingFrom)
	if err != nil {
		return fmt.Errorf("could not fetch %s builds: %s", r.builder1, err)
	}
	if len(builds1) == 0 {
		fmt.Printf("no %s builds\n", r.builder1)
		return nil
	}

	builds2, err := r.fetchBuilds(r.builder2, startingFrom)
	if err != nil {
		return fmt.Errorf("could not fetch %s builds: %s", r.builder2, err)
	}
	if len(builds2) == 0 {
		fmt.Printf("no %s builds\n", r.builder2)
		return nil
	}

	buildSets1 := groupBuilds(builds1)
	buildSets2 := groupBuilds(builds2)

	consistentN := 0
	inconsistentN := 0
	for setName, set2 := range buildSets2 {
		set1 := buildSets1[setName]
		if set1 == nil {
			fmt.Printf("no %s builds for buildset %s\n", r.builder1, setName)
			continue
		}
		if set1.bestResult == set2.bestResult {
			consistentN++
			continue
		}
		inconsistentN++

		fmt.Printf("%s is inconsistent\n", setName)
		for _, b := range set2.builds {
			fmt.Printf("  %s %s\n", b.Result, b.Url)
		}
		for _, b := range set1.builds {
			fmt.Printf("  %s %s\n", b.Result, b.Url)
		}
	}

	fmt.Printf("%0.2f%% consistent build sets, %d %s builds, %d %s builds\n",
		100*float64(consistentN)/float64(consistentN+inconsistentN),
		len(builds1), r.builder1,
		len(builds2), r.builder2)

	time1 := medianTime(builds1)
	time2 := medianTime(builds2)
	factor := float64(time1) / float64(time2)
	if factor >= 1 {
		fmt.Printf("%s is %.1fx faster\n", r.builder2, factor)
	} else {
		fmt.Printf("%s is %.1fx slower\n", r.builder2, 1/factor)
	}
	fmt.Printf("%s median time: %s\n", r.builder1, time1)
	fmt.Printf("%s median time: %s\n", r.builder2, time2)
	return nil
}

func (r *inconsistencyRun) fetchBuilds(builder builderID, startingFrom time.Time) ([]*buildbucket.ApiCommonBuildMessage, error) {
	req := r.client.Search()
	req.Bucket(builder.Bucket)
	req.Tag("builder:" + builder.Builder)
	req.Status("COMPLETED")
	req.MaxBuilds(100)

	var result []*buildbucket.ApiCommonBuildMessage
	for {
		res, err := req.Do()
		if err != nil {
			return result, err
		}
		if res.Error != nil {
			return result, fmt.Errorf(res.Error.Message)
		}

		for _, b := range res.Builds {
			if parseTimestamp(b.CreatedTs).Before(startingFrom) {
				return result, nil
			}
			result = append(result, b)
		}

		if len(res.Builds) == 0 || res.NextCursor == "" {
			break
		}
		req.StartCursor(res.NextCursor)
	}
	return result, nil
}

type buildSet struct {
	builds     []*buildbucket.ApiCommonBuildMessage
	bestResult string
}

// groupBuilds groups builds by buildset tag.
func groupBuilds(builds []*buildbucket.ApiCommonBuildMessage) map[string]*buildSet {
	results := map[string]*buildSet{}
	for _, b := range builds {
		tags := parseTags(b.Tags)
		buildSetName := tags["buildset"]
		if buildSetName == "" {
			fmt.Printf("skipped build %d: no buildset tag\n", b.Id)
			continue
		}
		set := results[buildSetName]
		if set == nil {
			set = &buildSet{}
			results[buildSetName] = set
		}

		set.builds = append(set.builds, b)
		if set.bestResult == "" || b.Result == "SUCCESS" {
			set.bestResult = b.Result
		}
	}
	return results
}

// medianTime returns median completed_time - created_time of successful builds.
func medianTime(builds []*buildbucket.ApiCommonBuildMessage) time.Duration {
	if len(builds) == 0 {
		return 0
	}
	durations := make(durationSlice, 0, len(builds))
	for _, b := range builds {
		if b.Result != "SUCCESS" {
			continue
		}
		created := parseTimestamp(b.CreatedTs)
		completed := parseTimestamp(b.CompletedTs)
		durations = append(durations, completed.Sub(created))
	}
	sort.Sort(durations)
	return durations[len(durations)/2]
}

func parseTags(tags []string) map[string]string {
	result := make(map[string]string, len(tags))
	for _, t := range tags {
		parts := strings.SplitN(t, ":", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}

func parseTimestamp(ts int64) time.Time {
	if ts == 0 {
		return time.Time{}
	}
	return time.Unix(ts/1000000, 0)
}

type durationSlice []time.Duration

func (a durationSlice) Len() int           { return len(a) }
func (a durationSlice) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a durationSlice) Less(i, j int) bool { return a[i] < a[j] }
