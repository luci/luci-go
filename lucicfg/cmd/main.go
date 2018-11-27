// Copyright 2018 The LUCI Authors.
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

// Command lucicfg is CLI for LUCI config generator.
package main

import (
	"context"
	"fmt"
	"os"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/lucicfg"
	"go.chromium.org/luci/starlark/interpreter"
)

// It's just a tiny example for now.

func main() {
	ctx := context.Background()

	state, err := lucicfg.Generate(ctx, lucicfg.Inputs{
		Code: interpreter.MemoryLoader(map[string]string{
			"LUCI.star": `say_hi("Hello, world")`,
		}),
		Entry: "LUCI.star",
	})

	if err != nil {
		if evalErr, _ := err.(*starlark.EvalError); evalErr != nil {
			fmt.Fprintf(os.Stderr, "%s\n", evalErr.Backtrace())
		} else {
			fmt.Fprintf(os.Stderr, "%s\n", err)
		}
		os.Exit(1)
	}

	for _, g := range state.Greetings {
		fmt.Printf("Starlark said: %s\n", g)
	}
}
