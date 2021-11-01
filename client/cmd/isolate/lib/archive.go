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

package lib

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/client/archiver/tarring"
	"go.chromium.org/luci/client/casclient"
	"go.chromium.org/luci/client/isolate"
	"go.chromium.org/luci/common/data/text/units"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/system/signals"
)

// CmdArchive returns an object for the `archive` subcommand.
func CmdArchive(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "archive <options>",
		ShortDesc: "parses a .isolate file to create a .isolated file, and uploads it and all referenced files to an isolate server",
		LongDesc:  "All the files listed in the .isolated file are put in the isolate server cache. Small files are combined together in a tar archive before uploading.",
		CommandRun: func() subcommands.CommandRun {
			c := archiveRun{}
			c.commonServerFlags.Init(defaultAuthOpts)
			c.isolateFlags.Init(&c.Flags)
			c.casFlags.Init(&c.Flags)
			c.Flags.StringVar(&c.Isolated, "isolated", "", ".isolated file to generate")
			c.Flags.StringVar(&c.Isolated, "s", "", "Alias for --isolated")
			c.Flags.IntVar(&c.maxConcurrentChecks, "max-concurrent-checks", 1, "The maximum number of in-flight check requests.")
			c.Flags.IntVar(&c.maxConcurrentUploads, "max-concurrent-uploads", 8, "The maximum number of in-flight uploads.")
			c.Flags.StringVar(&c.dumpJSON, "dump-json", "",
				"Write isolated digests of archived trees to this file as JSON")
			return &c
		},
	}
}

type archiveRun struct {
	commonServerFlags
	isolateFlags
	casFlags             casclient.Flags
	maxConcurrentChecks  int
	maxConcurrentUploads int
	dumpJSON             string
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
	if c.casFlags.UseCAS() {
		ctx, err := casclient.ContextWithMetadata(ctx, "isolate")
		if err != nil {
			return err
		}
		roots, err := c.uploadToCAS(ctx, c.dumpJSON, c.commonServerFlags.parsedAuthOpts, &c.casFlags, al, &c.ArchiveOptions)
		if err != nil {
			return err
		}
		al.Printf("uploaded digest: %s\n", roots[0])
		return nil
	}

	return c.archiveToIsolate(ctx, al)
}

// archiveToIsolate performs the archiveToIsolate operation for an isolate specified by opts.
// dumpJSON is the path to write a JSON summary of the uploaded isolate, in the same format as batch_archive.
func (c *archiveRun) archiveToIsolate(ctx context.Context, al *archiveLogger) error {
	authCl, err := c.createAuthClient(ctx)
	if err != nil {
		return err
	}
	client, err := c.createIsolatedClient(authCl)
	if err != nil {
		return err
	}

	opts := &c.ArchiveOptions
	// Parse the incoming isolate file.
	deps, rootDir, isol, err := isolate.ProcessIsolate(opts)
	if err != nil {
		return errors.Annotate(err, "isolate %s: failed to process", opts.Isolate).Err()
	}
	log.Printf("Isolate %s referenced %d deps", opts.Isolate, len(deps))

	// Set up a checker and uploader.
	checker := tarring.NewChecker(ctx, client, c.maxConcurrentChecks)
	uploader := tarring.NewUploader(ctx, client, c.maxConcurrentUploads)
	arc := tarring.NewArchiver(checker, uploader)
	isolSummary, err := arc.Archive(&tarring.ArchiveArgs{
		Deps:          deps,
		RootDir:       rootDir,
		IgnoredPathRe: opts.IgnoredPathFilterRe,
		Isolated:      opts.Isolated,
		Isol:          isol,
	})
	if err != nil {
		return errors.Annotate(err, "isolate %s", opts.Isolate).Err()
	}

	// Make sure that all pending items have been checked.
	if err := checker.Close(); err != nil {
		return err
	}

	// Make sure that all the uploads have completed successfully.
	if err := uploader.Close(); err != nil {
		return err
	}

	al.printSummary(isolSummary)
	if err := dumpSummaryJSON(c.dumpJSON, isolSummary); err != nil {
		return err
	}

	al.LogSummary(ctx, checker.Hit.Count(), checker.Miss.Count(), units.Size(checker.Hit.Bytes()), units.Size(checker.Miss.Bytes()))
	return nil
}

func (c *archiveRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	if err := c.Parse(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	if err := c.main(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), strings.Join(errors.RenderStack(err), "\n"))
		return 1
	}
	return 0
}

func dumpSummaryJSON(filename string, summaries ...tarring.IsolatedSummary) error {
	if len(filename) == 0 {
		return nil
	}
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	m := map[string]isolated.HexDigest{}
	for _, summary := range summaries {
		m[summary.Name] = summary.Digest
	}
	return json.NewEncoder(f).Encode(m)
}
