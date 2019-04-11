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
	"io/ioutil"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/starlark/interpreter"
)

var regen = flag.Bool("test.regen", false, "regenerated configs")

func TestExamples(t *testing.T) {
	t.Parallel()
	fi, err := ioutil.ReadDir("examples")
	if err != nil {
		t.Fatalf("ioutil.ReadDir: %s", err)
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
	abs, err := filepath.Abs(script)
	if err != nil {
		return err
	}

	root, main := filepath.Split(abs)
	output := filepath.Join(root, "generated")

	state, err := Generate(context.Background(), Inputs{
		Code:         interpreter.FileSystemLoader(root),
		Entry:        main,
		TextPBHeader: DefaultTextPBHeader,
	})
	if err != nil {
		return err
	}

	if *regen {
		_, _, err := state.Output.Write(output)
		return err
	}

	diff, _ := state.Output.Compare(output)
	if len(diff) != 0 {
		return fmt.Errorf("the following generated files are stale, run `go generate .`: %q", diff)
	}

	return nil
}
