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

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/google/skylark"
	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/skylark/interpreter"

	"go.chromium.org/luci/lucicfg/stdlib"
)

func cmdGenerate() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "gen [LUCI.sky] <options>",
		ShortDesc: "Reads LUCI.sky and generates LUCI config files based on it",
		CommandRun: func() subcommands.CommandRun {
			c := &generateCmd{}
			c.init()
			return c
		},
	}
}

type generateCmd struct {
	subcommands.CommandRunBase

	// Flags.
	stdlibDir string

	// State derived from flags in init().
	stdlibLoader func(string) (string, error)
	rootScript   string
}

func (c *generateCmd) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if err := c.parse(args, env); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	if err := c.main(ctx); err != nil {
		if evalErr, _ := err.(*skylark.EvalError); evalErr != nil {
			fmt.Fprintf(os.Stderr, "%s\n", evalErr.Backtrace())
		} else {
			fmt.Fprintf(os.Stderr, "%s\n", err)
		}
		return 2
	}
	return 0
}

func (c *generateCmd) init() {
	c.Flags.StringVar(&c.stdlibDir, "stdlib", "", "directory with init.sky to preload instead of the hardcoded one")
}

func (c *generateCmd) parse(args []string, env subcommands.Env) error {
	if len(args) > 1 {
		return fmt.Errorf("expecting at most one position argument, got %d", len(args))
	}

	// Build an appropriate stdlib loader.
	if c.stdlibDir == "" {
		hardcoded := stdlib.Assets()
		c.stdlibLoader = func(path string) (string, error) {
			if body, ok := hardcoded[path]; ok {
				return body, nil
			}
			return "", interpreter.ErrNoStdlibModule
		}
	} else {
		c.stdlibLoader = func(path string) (string, error) {
			path = filepath.Join(c.stdlibDir, filepath.FromSlash(path))
			body, err := ioutil.ReadFile(path)
			if os.IsNotExist(err) {
				return "", interpreter.ErrNoStdlibModule
			}
			return string(body), nil
		}
	}

	// Find the root LUCI.sky script.
	if len(args) == 1 {
		var err error
		if c.rootScript, err = filepath.Abs(args[0]); err != nil {
			return errors.Annotate(err, "failed to get abs path to %q", args[0]).Err()
		}
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			return errors.Annotate(err, "failed to get cwd").Err()
		}
		for {
			c.rootScript = filepath.Join(cwd, "LUCI.sky")
			if _, err := os.Stat(c.rootScript); err == nil {
				break
			}
			up := filepath.Dir(cwd)
			if up == cwd {
				return errors.Reason("can't find LUCI.sky in cwd or any of its parents").Err()
			}
			cwd = up
		}
	}

	return nil
}

func (c *generateCmd) main(ctx context.Context) error {
	gen := Generator{}
	intr := interpreter.Interpreter{
		Root:        filepath.Dir(c.rootScript),
		Stdlib:      c.stdlibLoader,
		Predeclared: gen.ExportedBuiltins(),
	}
	if err := intr.Init(); err != nil {
		return err
	}
	if _, err := intr.ExecFile(c.rootScript); err != nil {
		return err
	}
	gen.Report()
	return nil
}
