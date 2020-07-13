// Copyright 2020 The LUCI Authors.
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

package lib

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cas"
	"go.chromium.org/luci/client/isolate"
	"go.chromium.org/luci/common/system/signals"
)

// CmdCASArchive returns an object for the `casarchive` subcommand.
func CmdCASArchive() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "casarchive <options>",
		ShortDesc: "parses a .isolate file to create a .isolated file, and uploads it and all referenced files to CAS server",
		LongDesc:  "All the files listed in the .isolated file are put in the isolate server cache. Small files are combined together in a tar archive before uploading.",
		CommandRun: func() subcommands.CommandRun {
			c := casArchiveRun{}
			c.commonFlags.Init()
			c.isolateFlags.Init(&c.Flags)
			c.loggingFlags.Init(&c.Flags)
			c.Flags.StringVar(&c.Isolated, "isolated", "", ".isolated file to generate")
			c.Flags.StringVar(&c.Isolated, "s", "", "Alias for --isolated")
			c.Flags.StringVar(&c.dumpJSON, "dump-json", "",
				"Write isolated digests of archived trees to this file as JSON")
			return &c
		},
	}
}

type casArchiveRun struct {
	commonFlags
	isolateFlags
	loggingFlags loggingFlags
	dumpJSON     string
}

func (c *casArchiveRun) Parse(a subcommands.Application, args []string) error {
	if err := c.commonFlags.Parse(); err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := c.isolateFlags.Parse(cwd, RequireIsolateFile); err != nil {
		return err
	}
	if len(args) != 0 {
		return errors.New("position arguments not expected")
	}
	return nil
}

func (c *casArchiveRun) main(a subcommands.Application, args []string) error {
	start := time.Now()
	ctx, cancel := context.WithCancel(c.defaultFlags.MakeLoggingContext(os.Stderr))
	signals.HandleInterrupt(cancel)
	al := archiveLogger{
		start: start,
		quiet: c.defaultFlags.Quiet,
	}

	return casArchive(ctx, &c.ArchiveOptions, c.dumpJSON, al)
}

// casArchive performs the archive operation for an isolate specified by opts.
// dumpJSON is the path to write a JSON summary of the uploaded isolate, in the same format as batch_archive.
func casArchive(ctx context.Context, opts *isolate.ArchiveOptions, dumpJSON string, al archiveLogger) error {
	casCl, err := client.NewClient(ctx, "projects/chromium-swarm-dev/instances/default_instance",
		client.DialParams{
			Service:               "remotebuildexecution.googleapis.com:443",
			UseApplicationDefault: true,
		})
	if err != nil {
		return err
	}
	uploader := cas.NewUploader(ctx, casCl)
	rootDg, err := uploader.Upload(opts)
	al.Printf("rootDg=%v\n", rootDg)
	return err
}

func (c *casArchiveRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
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
