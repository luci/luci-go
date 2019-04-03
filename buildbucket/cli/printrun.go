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
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/status"

	pb "go.chromium.org/luci/buildbucket/proto"
)

var completeBuildFieldMask *field_mask.FieldMask
var idFieldMask = &field_mask.FieldMask{Paths: []string{"id"}}

func init() {
	completeBuildFieldMask = &field_mask.FieldMask{}
	for _, p := range proto.GetProperties(reflect.TypeOf(pb.Build{})).Prop {
		if !strings.HasPrefix(p.OrigName, "XXX") {
			completeBuildFieldMask.Paths = append(completeBuildFieldMask.Paths, p.OrigName)
		}
	}
}

// printRun is a base command run for subcommands that print
// builds.
type printRun struct {
	baseCommandRun
	all        bool
	properties bool
	steps      bool
	id         bool
}

func (r *printRun) RegisterDefaultFlags(p Params) {
	r.baseCommandRun.RegisterDefaultFlags(p)
	r.baseCommandRun.RegisterJSONFlag()

	r.Flags.BoolVar(&r.id, "id", false, doc(`
		Print only build ids.

		Intended for piping the output into another bb subcommand.
	`))
}

// RegisterFieldFlags registers -A, -steps and -p flags.
func (r *printRun) RegisterFieldFlags() {
	r.Flags.BoolVar(&r.all, "A", false, "Print build entirely")
	r.Flags.BoolVar(&r.steps, "steps", false, "Print steps")
	r.Flags.BoolVar(&r.properties, "p", false, "Print input/output properties")
}

// FieldMask returns the field mask to use in buildbucket requests.
func (r *printRun) FieldMask() (*field_mask.FieldMask, error) {
	if r.id {
		if r.all || r.properties || r.steps {
			return nil, fmt.Errorf("-id is mutually exclusive with -A, -p and -steps")
		}
		return proto.Clone(idFieldMask).(*field_mask.FieldMask), nil
	}

	if r.all {
		if r.properties || r.steps {
			return nil, fmt.Errorf("-A is mutually exclusive with -p and -steps")
		}
		return proto.Clone(completeBuildFieldMask).(*field_mask.FieldMask), nil
	}

	ret := &field_mask.FieldMask{
		Paths: []string{
			"builder",
			"create_time",
			"created_by",
			"end_time",
			"id",
			"input.experimental",
			"input.gerrit_changes",
			"input.gitiles_commit",
			"number",
			"start_time",
			"status",
			"status_details",
			"summary_markdown",
			"tags",
			"update_time",
		},
	}

	if r.properties {
		ret.Paths = append(ret.Paths, "input.properties", "output.properties")
	}

	if r.steps {
		ret.Paths = append(ret.Paths, "steps")
	}

	return ret, nil
}

func (r *printRun) printBuild(p *printer, build *pb.Build, first bool) {
	if r.json {
		if r.id {
			p.f(`{"id": "%d"}`, build.Id)
			p.f("\n")
		} else {
			p.JSONPB(build)
		}
	} else {
		if r.id {
			p.f("%d\n", build.Id)
		} else {
			if !first {
				p.f("\n")
			}
			p.Build(build)
		}
	}
}

// printAndDone executes request for each argument, prints builds and
// return exit code.
// request is called concurrently, but builds are printed in the same order
// as args.
func (r *printRun) printAndDone(args []string, request func(string) (*pb.Build, error)) int {
	stdout := newPrinter(os.Stdout, r.noColor, time.Now)
	stderr := newPrinter(os.Stderr, r.noColor, time.Now)

	// Prepare workspace.
	type result struct {
		*pb.Build
		err error
	}
	type workItem struct {
		arg    string
		result chan result
	}
	work := make(chan workItem)
	results := make(chan workItem, 256)

	// Prepare concurrent workers.
	var wg sync.WaitGroup
	const workers = 10
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range work {
				build, err := request(item.arg)
				item.result <- result{build, err}
			}
		}()
	}

	// Add work. Close the work space when work is
	go func() {
		defer close(results)
		for a := range argChan(args) {
			item := workItem{arg: a, result: make(chan result)}
			work <- item
			results <- item
		}
		close(work)
		wg.Wait()
	}()

	// Print builds in the order of args.
	first := true
	perfect := true
	for i := range results {
		res := <-i.result
		if res.err != nil {
			perfect = false
			if !first {
				stderr.f("\n")
			}
			st, _ := status.FromError(res.err)
			stderr.f("arg %q: %s\n", i.arg, st.Message())
			continue
		}

		r.printBuild(stdout, res.Build, first)
		first = false
	}
	if !perfect {
		return 1
	}
	return 0
}
