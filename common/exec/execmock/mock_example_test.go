// Copyright 2023 The LUCI Authors.
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

package execmock_test

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/exec"
	"go.chromium.org/luci/common/exec/execmock"
)

func TestMain(m *testing.M) {
	execmock.Intercept(true)
	os.Exit(m.Run())
}

// CustomInput is data that the TEST wants to communicate to CustomRunner.
//
// This should typically be things which affect the behavior of the Runner, e.g.
// to simulate different outputs, errors, etc., or to change the data that the
// Runner responds with.
type CustomInput struct {
	// OutputFile is a string that the test sends to CustomRunner to have it write
	// out to the `--output` argument.
	OutputFile []byte

	// CollectInput indicates that the test wants the Runner to collect the data
	// of the file specified by the `--input` argument.
	CollectInput bool
}

type CustomOutput struct {
	// If CollectInput was true, expect and read the input file and return the
	// data here.
	InputData []byte
}

var CustomRunner = execmock.Register(func(in *CustomInput) (*CustomOutput, int, error) {
	input := flag.String("input", "", "")
	_ = flag.String("random", "", "")
	output := flag.String("output", "", "")
	flag.Parse()

	ret := &CustomOutput{}

	if in.CollectInput {
		if *input == "" {
			return nil, 1, errors.New("input was expected")
		}
		data, err := os.ReadFile(*input)
		if err != nil {
			return nil, 1, errors.Fmt("reading input file: %w", err)
		}
		ret.InputData = data
	}

	if in.OutputFile != nil {
		if err := os.WriteFile(*output, in.OutputFile, 0777); err != nil {
			return nil, 1, errors.Fmt("writing output file: %w", err)
		}
	}

	return ret, 0, nil
})

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func ExampleInit() {
	// See this file for the definition of `CustomRunner`

	// BEGIN: TEST CODE
	ctx := execmock.Init(context.Background())

	outputUses := CustomRunner.WithArgs("--output").Mock(ctx, &CustomInput{
		OutputFile: []byte("hello I am Mx. Catopolous"),
	})
	allOtherUses := CustomRunner.Mock(ctx)
	inputUses := CustomRunner.WithArgs("--input").Mock(ctx, &CustomInput{
		CollectInput: true,
	})
	tmpDir, err := os.MkdirTemp("", "")
	must(errors.WrapIf(err, "failed to create tmpDir"))

	defer func() {
		must(os.RemoveAll(tmpDir))
	}()
	// END: TEST CODE

	// BEGIN: APPLICATION CODE
	// Your program would then get `ctx` passed to it from the test, and it would
	// run commands as usual:

	// this should be intercepted by `allOtherUses`
	must(exec.Command(ctx, "some_prog", "--random", "argument").Run())
	fmt.Println("[some_prog --random argument]: OK")

	// this should be intercepted by `inputUses`
	inputFile := filepath.Join(tmpDir, "input_file")
	must(os.WriteFile(inputFile, []byte("hello world"), 0777))
	must(exec.Command(ctx, "another_program", "--input", inputFile).Run())
	fmt.Println("[another_program --input inputFile]: OK")

	// this should be intercepted by `outputUses`.
	outputFile := filepath.Join(tmpDir, "output_file")
	outputCall := exec.Command(ctx, "another_program", "--output", outputFile)
	outputCall.Stdout = os.Stdout
	outputCall.Stderr = os.Stderr
	must(outputCall.Run())

	outputFileData, err := os.ReadFile(outputFile)
	must(errors.WrapIf(err, "our mock failed to write %q", outputFile))
	if !bytes.Equal(outputFileData, []byte("hello I am Mx. Catopolous")) {
		panic(errors.New("our mock failed to write the expected data"))
	}
	fmt.Printf("[another_program --output outputFile]: %q\n", outputFileData)
	// END: APPLICATION CODE

	// BEGIN: TEST CODE
	// Back in the test after running the application code. Now we can look and
	// see what our mocks caught.
	fmt.Printf("allOtherUses: got %d calls\n", len(allOtherUses.Snapshot()))
	fmt.Printf("outputUses: got %d calls\n", len(outputUses.Snapshot()))

	incallMock, _, err := inputUses.Snapshot()[0].GetOutput(ctx)
	must(errors.WrapIf(err, "could not get output from --input call"))
	fmt.Printf("incallMock: saw %q written to the mock program\n", incallMock.InputData)
	// END: TEST CODE

	// Output:
	// [some_prog --random argument]: OK
	// [another_program --input inputFile]: OK
	// [another_program --output outputFile]: "hello I am Mx. Catopolous"
	// allOtherUses: got 1 calls
	// outputUses: got 1 calls
	// incallMock: saw "hello world" written to the mock program
}
