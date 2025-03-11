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

package isolateimpl

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/client/casclient"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/signals"
)

// CmdArchive returns an object for the `archive` subcommand.
func CmdArchive(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "archive <options>",
		ShortDesc: "parses a .isolate file and uploads all referenced files to a CAS server",
		LongDesc:  "All the files referenced from .isolate are put in the CAS server cache.",
		CommandRun: func() subcommands.CommandRun {
			c := archiveRun{}
			c.commonServerFlags.Init(defaultAuthOpts)
			c.isolateFlags.Init(&c.Flags)
			c.casFlags.Init(&c.Flags)
			c.Flags.StringVar(&c.dumpJSON, "dump-json", "",
				"Write isolated digests of archived trees to this file as JSON")
			return &c
		},
	}
}

type archiveRun struct {
	commonServerFlags
	isolateFlags
	casFlags casclient.Flags
	dumpJSON string
}

func (c *archiveRun) Parse(a subcommands.Application, args []string) error {
	if err := c.commonServerFlags.Parse(); err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := c.isolateFlags.Parse(cwd); err != nil {
		return err
	}
	if err := c.casFlags.Parse(); err != nil {
		return err
	}
	if len(args) != 0 {
		return errors.Reason("position arguments not expected").Err()
	}
	return nil
}

func (c *archiveRun) main(a subcommands.Application, args []string) error {
	start := time.Now()
	ctx, cancel := context.WithCancel(c.defaultFlags.MakeLoggingContext(os.Stderr))
	defer cancel()
	defer signals.HandleInterrupt(cancel)()

	al := &archiveLogger{
		start: start,
		quiet: c.defaultFlags.Quiet,
	}
	ctx, err := casclient.ContextWithMetadata(ctx, "isolate")
	if err != nil {
		return err
	}
	_, err = c.uploadToCAS(ctx, c.dumpJSON, c.commonServerFlags.parsedAuthOpts, &c.casFlags, al, &c.ArchiveOptions)
	return err
}

func (c *archiveRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	if err := c.Parse(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	if err := c.main(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), errors.RenderStack(err))
		return 1
	}
	return 0
}
