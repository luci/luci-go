// Copyright 2026 The LUCI Authors.
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
	"strings"
	"time"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"

	"go.chromium.org/luci/cipd/client/cipd"
)

////////////////////////////////////////////////////////////////////////////////
// 'instances' subcommand.

func cmdInstances(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "instances <package> [-limit ...]",
		ShortDesc: "lists instances of a package",
		LongDesc:  "Lists instances of a package, most recently uploaded first.",
		CommandRun: func() subcommands.CommandRun {
			c := &instancesRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.IntVar(&c.limit, "limit", 20, "How many instances to return or 0 for all.")
			return c
		},
	}
}

type instancesRun struct {
	cipdSubcommand
	clientOptions

	limit int
}

func (c *instancesRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(listInstances(ctx, args[0], c.limit, c.clientOptions))
}

type instanceInfoWithRefs struct {
	cipd.InstanceInfo
	Refs []string `json:"refs,omitempty"`
}

// instancesOutput defines JSON format of 'cipd instances' output.
type instancesOutput struct {
	Instances []instanceInfoWithRefs `json:"instances"`
}

func listInstances(ctx context.Context, pkg string, limit int, clientOpts clientOptions) (*instancesOutput, error) {
	pkg, err := expandTemplate(pkg)
	if err != nil {
		return nil, err
	}

	client, err := clientOpts.makeCIPDClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close(ctx)

	// TODO(vadimsh): The backend currently doesn't support retrieving
	// per-instance refs when listing instances. Instead we fetch ALL refs in
	// parallel and then merge this information with the instance listing. This
	// works fine for packages with few refs (up to 50 maybe), but horribly
	// inefficient if the cardinality of the set of all refs is larger than a
	// typical size of instance listing (we spend time fetching data we don't
	// need). To support this case better, the backend should learn to maintain
	// {instance ID => ref} mapping (in addition to {ref => instance ID} mapping
	// it already has). This would require back filling all existing entities.

	// Fetch the refs in parallel with the first page of results. We merge them
	// with the list of instances during the display.
	type refsMap map[string][]string // instance ID => list of refs
	type refsOrErr struct {
		refs refsMap
		err  error
	}
	refsChan := make(chan refsOrErr, 1)
	go func() {
		defer close(refsChan)
		asMap := refsMap{}
		refs, err := client.FetchPackageRefs(ctx, pkg)
		for _, info := range refs {
			asMap[info.InstanceID] = append(asMap[info.InstanceID], info.Ref)
		}
		refsChan <- refsOrErr{asMap, err}
	}()

	enum, err := client.ListInstances(ctx, pkg)
	if err != nil {
		return nil, err
	}

	formatRow := func(instanceID, when, who, refs string) string {
		if len(who) > 25 {
			who = who[:22] + "..."
		}
		return fmt.Sprintf("%-44s │ %-21s │ %-25s │ %-12s", instanceID, when, who, refs)
	}

	var refs refsMap // populated on after fetching first page

	out := []instanceInfoWithRefs{}
	for {
		pageSize := 200
		if limit != 0 && limit-len(out) < pageSize {
			pageSize = limit - len(out)
			if pageSize == 0 {
				// Fetched everything we wanted. There's likely more instances available
				// (unless '-limit' happens to exactly match number of instances on the
				// backend, which is not very probable). Hint this by printing '...'.
				fmt.Println(formatRow("...", "...", "...", "..."))
				break
			}
		}
		page, err := enum.Next(ctx, pageSize)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			if len(out) == 0 {
				fmt.Println("No instances found")
			}
			break // no more results to fetch
		}

		if len(out) == 0 {
			// Need to wait for refs to be fetched, they are required to display
			// "Refs" column.
			refsOrErr := <-refsChan
			if refsOrErr.err != nil {
				return nil, refsOrErr.err
			}
			refs = refsOrErr.refs

			// Draw the header now that we have some real results (i.e no errors).
			hdr := formatRow("Instance ID", "Timestamp", "Uploader", "Refs")
			fmt.Println(hdr)
			fmt.Println(strings.Repeat("─", len(hdr)))
		}

		for _, info := range page {
			instanceRefs := refs[info.Pin.InstanceID]
			out = append(out, instanceInfoWithRefs{
				InstanceInfo: info,
				Refs:         instanceRefs,
			})
			fmt.Println(formatRow(
				info.Pin.InstanceID,
				time.Time(info.RegisteredTs).Local().Format("Jan 02 15:04 MST 2006"),
				strings.TrimPrefix(info.RegisteredBy, "user:"),
				strings.Join(instanceRefs, " ")))
		}
	}

	return &instancesOutput{out}, nil
}
