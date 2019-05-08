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

	"github.com/golang/protobuf/proto"
	"google.golang.org/genproto/protobuf/field_mask"

	pb "go.chromium.org/luci/buildbucket/proto"
)

var idFieldMask = &field_mask.FieldMask{Paths: []string{"id"}}

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
}

// RegisterIDFlag registers -id flag.
func (r *printRun) RegisterIDFlag() {
	r.Flags.BoolVar(&r.id, "id", false, doc(`
		Print only build ids.

		Intended for piping the output into another bb subcommand:
			bb ls -cl myCL -id | bb cancel
	`))
}

// RegisterFieldFlags registers -A, -steps and -p flags.
func (r *printRun) RegisterFieldFlags() {
	r.Flags.BoolVar(&r.all, "A", false, doc(`
		Print builds in their entirety.
		With -json, prints all build fields.
		Without -json, implies -steps and -p.
	`))
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
		return &field_mask.FieldMask{Paths: []string{"*"}}, nil
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

func (r *printRun) printBuild(p *printer, build *pb.Build, first bool) error {
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
				// Print a new line so it is easier to differentiate builds.
				p.f("\n")
			}
			p.Build(build)
		}
	}
	return p.Err
}

// PrintAndDone calls fn for each argument, prints builds and returns exit code.
// fn is called concurrently, but builds are printed in the same order
// as args.
func (r *printRun) PrintAndDone(ctx context.Context, args []string, fn func(context.Context, string) (*pb.Build, error)) int {
	stdout, stderr := newStdioPrinters(r.noColor)
	return r.printAndDone(ctx, stdout, stderr, args, fn)
}

func (r *printRun) printAndDone(ctx context.Context, stdout, stderr *printer, args []string, fn func(context.Context, string) (*pb.Build, error)) int {
	// Prepare workspace.
	type workItem struct {
		arg   string
		build *pb.Build
		done  chan error
	}
	work := make(chan *workItem)
	results := make(chan *workItem, 256)

	// Prepare 16 concurrent workers.
	for i := 0; i < 16; i++ {
		go func() {
			for item := range work {
				var err error
				item.build, err = fn(ctx, item.arg)
				item.done <- err
			}
		}()
	}

	// Add work. Close the work space when work is done.
	go func() {
		for a := range argChan(args) {
			if ctx.Err() != nil {
				break
			}
			item := &workItem{arg: a, done: make(chan error)}
			work <- item
			results <- item
		}
		close(work)
		close(results)
	}()

	// Print the results in the order of args.
	first := true
	perfect := true
	for i := range results {
		err := <-i.done
		if err != nil {
			perfect = false
			if !first {
				stderr.f("\n")
			}
			stderr.f("arg %q: ", i.arg)
			stderr.Error(err)
			stderr.f("\n")
			if stderr.Err != nil {
				return r.done(ctx, stderr.Err)
			}
		} else {
			if err := r.printBuild(stdout, i.build, first); err != nil {
				return r.done(ctx, err)
			}
		}
		first = false
	}
	if !perfect {
		return 1
	}
	return 0
}

// argChan returns a channel of args.
//
// If args is empty, reads from stdin. Trims whitespace and skips blank lines.
// Panics if reading from stdin fails.
func argChan(args []string) chan string {
	ret := make(chan string)
	go func() {
		defer close(ret)

		if len(args) > 0 {
			for _, a := range args {
				ret <- strings.TrimSpace(a)
			}
			return
		}

		reader := bufio.NewReader(os.Stdin)
		for {
			line, err := reader.ReadString('\n')
			line = strings.TrimSpace(line)
			switch {
			case err == io.EOF:
				return
			case err != nil:
				panic(err)
			case len(line) == 0:
				continue
			default:
				ret <- line
			}
		}
	}()
	return ret
}
