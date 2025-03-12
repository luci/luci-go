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

//go:generate go test . -run TestExamples -test.regen
package lucicfg

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/lucicfg/pkg"
)

var regen = flag.Bool("test.regen", false, "regenerated configs")

func TestExamples(t *testing.T) {
	t.Parallel()
	fi, err := os.ReadDir("examples")
	if err != nil {
		t.Fatalf("os.ReadDir: %s", err)
	}
	for _, f := range fi {
		if f.IsDir() {
			mainStar := filepath.Join("examples", f.Name(), "main.star")
			t.Run(f.Name(), func(t *testing.T) {
				t.Parallel()
				if err := runExample(mainStar); err != nil {
					t.Fatalf("%s", err)
				}
			})
		}
	}
}

func runExample(script string) error {
	entry, err := pkg.EntryOnDisk(context.Background(), script)
	if err != nil {
		return err
	}

	output := filepath.Join(entry.Local.DiskPath, "generated")

	state, err := Generate(context.Background(), Inputs{
		Entry:       entry,
		Meta:        &Meta{ConfigDir: "generated"},
		testVersion: "1.1.1",
	})
	if err != nil {
		return err
	}

	if *regen {
		_, _, err := state.Output.Write(output, true)
		return err
	}

	// We want examples in examples/... to be formatted using current "golden"
	// style, so do byte-to-byte comparison (not a semantic one).
	cmp, err := state.Output.Compare(output, false)
	if err != nil {
		return err
	}
	var diff []string
	for name, res := range cmp {
		if res == Different {
			diff = append(diff, name)
		}
	}
	if len(diff) != 0 {
		return fmt.Errorf("the following generated files are stale, run `go generate .`: %q", diff)
	}
	return nil
}
