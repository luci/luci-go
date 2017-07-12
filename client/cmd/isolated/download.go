// Copyright 2015 The LUCI Authors.
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
	"errors"
	"fmt"

	"github.com/luci/luci-go/common/auth"
	"github.com/maruel/subcommands"
)

func cmdDownload(authOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "download <options>...",
		ShortDesc: "downloads a file or a .isolated tree from an isolate server.",
		LongDesc: `Downloads one or multiple files, or a isolated tree from the isolate server.

Files are referenced by their hash`,
		CommandRun: func() subcommands.CommandRun {
			c := downloadRun{}
			c.commonFlags.Init(authOpts)
			return &c
		},
	}
}

type downloadRun struct {
	commonFlags
}

func (c *downloadRun) Parse(a subcommands.Application, args []string) error {
	if err := c.commonFlags.Parse(); err != nil {
		return err
	}
	if len(args) != 0 {
		return errors.New("position arguments not expected")
	}
	return nil
}

func (c *downloadRun) main(a subcommands.Application, args []string) error {
	return errors.New("TODO")
}

func (c *downloadRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	if err := c.Parse(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	cl, err := c.defaultFlags.StartTracing()
	if err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	defer cl.Close()
	if err := c.main(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}
