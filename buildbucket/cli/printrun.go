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

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/sync/parallel"

	pb "go.chromium.org/luci/buildbucket/proto"
)

var idFieldMask = &field_mask.FieldMask{Paths: []string{"id"}}
var allFieldMask = &field_mask.FieldMask{Paths: []string{"*"}}
var defaultFieldMask = &field_mask.FieldMask{
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

// extraFields are fields that will be printed even if not specified
// in the `-field` flag value.
var extraFields = []string{
	"id",
	"status",
	"builder",
}
var extraFieldsStr = strings.Join(extraFields, ", ")

// printRun is a base command run for subcommands that print
// builds.
type printRun struct {
	baseCommandRun
	all        bool
	properties bool
	steps      bool
	id         bool
	fields     string
	eager      bool
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

// RegisterFieldFlags registers -A, -steps, -p and -field flags.
func (r *printRun) RegisterFieldFlags() {
	r.Flags.BoolVar(&r.all, "A", false, doc(`
		Print builds in their entirety.
		With -json, prints all build fields.
		Without -json, implies -steps and -p.
	`))
	r.Flags.BoolVar(&r.steps, "steps", false, "Print steps")
	r.Flags.BoolVar(&r.properties, "p", false, "Print input/output properties")
	r.Flags.StringVar(&r.fields, "fields", "", doc(fmt.Sprintf(`
		Print only provided fields. Fields should be passed as a comma separated
		string to match the JSON encoding schema of FieldMask. Fields: [%s] will
		also be printed for better result readability even if not requested.

		This flag is mutually exclusive with -A, -p, -steps and -id.

		See: https://developers.google.com/protocol-buffers/docs/proto3#json
	`, extraFieldsStr)))
	r.Flags.BoolVar(&r.eager, "eager", false, "return upon the first finished build")
}

// FieldMask returns the field mask to use in buildbucket requests.
func (r *printRun) FieldMask() (*field_mask.FieldMask, error) {
	if err := r.validateFieldFlags(); err != nil {
		return nil, err
	}

	switch {
	case r.id:
		return proto.Clone(idFieldMask).(*field_mask.FieldMask), nil
	case r.all:
		return proto.Clone(allFieldMask).(*field_mask.FieldMask), nil
	case r.fields != "":
		// TODO(crbug/1039823): Use Unmarshal feature in JSONPB when protobuf v2
		// API is released. Currently, there's an existing issue in Go JSONPB
		// implementation which results in serialization and deserialization of
		// FieldMask not working as expected.
		// See: https://github.com/golang/protobuf/issues/745
		pathSet := stringset.NewFromSlice(strings.Split(r.fields, ",")...)
		pathSet.AddAll(extraFields)
		return &field_mask.FieldMask{Paths: pathSet.ToSortedSlice()}, nil
	default:
		ret := proto.Clone(defaultFieldMask).(*field_mask.FieldMask)
		if r.properties {
			ret.Paths = append(ret.Paths, "input.properties", "output.properties")
		}
		if r.steps {
			ret.Paths = append(ret.Paths, "steps")
		}
		return ret, nil
	}
}

// validateFieldFlags validates the combination of provided field flags.
func (r *printRun) validateFieldFlags() error {
	switch {
	case r.fields != "" && (r.all || r.properties || r.steps || r.id):
		return fmt.Errorf("-fields is mutually exclusive with -A, -p, -steps and -id")
	case r.id && (r.all || r.properties || r.steps):
		return fmt.Errorf("-id is mutually exclusive with -A, -p and -steps")
	case r.all && (r.properties || r.steps):
		return fmt.Errorf("-A is mutually exclusive with -p and -steps")
	default:
		return nil
	}
}

func (r *printRun) printBuild(p *printer, build *pb.Build, first bool) error {
	if r.json {
		if r.id {
			p.f(`{"id": "%d"}`, build.Id)
			p.f("\n")
		} else {
			p.JSONPB(build, true)
		}
	} else {
		if r.id {
			p.f("%d\n", build.Id)
		} else {
			if !first {
				// Print a new line so it is easier to differentiate builds.
				p.f("\n")
			}
			p.Build(build, r.host)
		}
	}
	return p.Err
}

type runOrder int

const (
	unordered runOrder = iota
	argOrder
)

// PrintAndDone calls fn for each argument, prints builds and returns exit code.
// fn is called concurrently, but builds are printed in the same order
// as args.
func (r *printRun) PrintAndDone(ctx context.Context, args []string, order runOrder, fn buildFunc) int {
	stdout, stderr := newStdioPrinters(r.noColor)

	jobs := len(args)
	if jobs == 0 {
		jobs = 32
	}

	resultC := make(chan buildResult, 256)
	go func() {
		defer close(resultC)
		argC := argChan(args)
		if order == argOrder {
			r.runOrdered(ctx, jobs, argC, resultC, fn)
		} else {
			r.runUnordered(ctx, jobs, argC, resultC, fn)
		}
	}()

	// Print the results in the order of args.
	first := true
	perfect := true
	for res := range resultC {
		if res.err != nil {
			perfect = false
			if !first {
				stderr.f("\n")
			}
			stderr.f("arg %q: ", res.arg)
			stderr.Error(res.err)
			stderr.f("\n")
			if stderr.Err != nil {
				return r.done(ctx, stderr.Err)
			}
		} else {
			if err := r.printBuild(stdout, res.build, first); err != nil {
				return r.done(ctx, err)
			}
		}
		first = false
		if r.eager {
			// return upon the first build.
			if !perfect {
				return 1
			}
			return 0
		}
	}
	if !perfect {
		return 1
	}
	return 0
}

type buildResult struct {
	arg   string
	build *pb.Build
	err   error
}

type buildFunc func(c context.Context, arg string) (*pb.Build, error)

// runOrdered runs fn for each arg in argC and reports results to resultC
// in the same order.
func (r *printRun) runOrdered(ctx context.Context, jobs int, argC <-chan string, resultC chan<- buildResult, fn buildFunc) {
	// Prepare workspace.
	type workItem struct {
		arg   string
		build *pb.Build
		done  chan error
	}
	work := make(chan *workItem)

	// Prepare concurrent workers.
	for i := 0; i < jobs; i++ {
		go func() {
			for item := range work {
				var err error
				item.build, err = fn(ctx, item.arg)
				item.done <- err
			}
		}()
	}

	// Add work. Close the workspace when the work is done.
	resultItems := make(chan *workItem)
	go func() {
		for a := range argC {
			item := &workItem{arg: a, done: make(chan error)}
			work <- item
			resultItems <- item
		}
		close(work)
		close(resultItems)
	}()

	for i := range resultItems {
		resultC <- buildResult{
			arg:   i.arg,
			build: i.build,
			err:   <-i.done,
		}
	}
}

// runUnordered is like runOrdered, but unordered.
func (r *printRun) runUnordered(ctx context.Context, jobs int, argC <-chan string, resultC chan<- buildResult, fn buildFunc) {
	parallel.WorkPool(jobs, func(work chan<- func() error) {
		for arg := range argC {
			if ctx.Err() != nil {
				break
			}
			work <- func() error {
				build, err := fn(ctx, arg)
				resultC <- buildResult{arg, build, err}
				return nil
			}
		}
	})
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
