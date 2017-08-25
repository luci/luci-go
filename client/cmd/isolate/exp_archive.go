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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"go.chromium.org/luci/client/isolate"
	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/data/text/units"
	logpb "go.chromium.org/luci/common/eventlog/proto"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
)

const (
	// archiveThreshold is the size (in bytes) used to determine whether to add
	// files to a tar archive before uploading. Files smaller than this size will
	// be combined into archives before being uploaded to the server.
	archiveThreshold = 100e3 // 100kB

	// archiveMaxSize is the maximum size of the created archives.
	archiveMaxSize = 10e6

	// infraFailExit is the exit code used when the exparchive fails due to
	// infrastructure errors (for example, failed server requests).
	infraFailExit = 2
)

func cmdExpArchive(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "exparchive <options>",
		ShortDesc: "EXPERIMENTAL parses a .isolate file to create a .isolated file, and uploads it and all referenced files to an isolate server",
		LongDesc:  "All the files listed in the .isolated file are put in the isolate server cache. Small files are combined together in a tar archive before uploading.",
		CommandRun: func() subcommands.CommandRun {
			c := &expArchiveRun{}
			c.commonServerFlags.Init(defaultAuthOpts)
			c.isolateFlags.Init(&c.Flags)
			c.loggingFlags.Init(&c.Flags)
			c.Flags.StringVar(&c.dumpJSON, "dump-json", "",
				"Write isolated digests of archived trees to this file as JSON")
			return c
		},
	}
}

// expArchiveRun contains the logic for the experimental archive subcommand.
// It implements subcommand.CommandRun
type expArchiveRun struct {
	commonServerFlags // Provides the GetFlags method.
	isolateFlags      isolateFlags
	loggingFlags      loggingFlags
	dumpJSON          string
}

// main contains the core logic for experimental archive.
func (c *expArchiveRun) main() error {
	start := time.Now()
	archiveOpts := &c.isolateFlags.ArchiveOptions

	// Set up a background context which is cancelled when this function returns.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create the isolated client which connects to the isolate server.
	authCl, err := c.createAuthClient()
	if err != nil {
		return err
	}
	client := isolatedclient.New(nil, authCl, c.isolatedFlags.ServerURL, c.isolatedFlags.Namespace, nil, nil)

	al := archiveLogger{
		logger:    NewLogger(ctx, c.loggingFlags.EventlogEndpoint),
		operation: logpb.IsolateClientEvent_ARCHIVE.Enum(),
		start:     start,
		quiet:     c.commonServerFlags.commonFlags.defaultFlags.Quiet,
	}

	return doExpArchive(ctx, client, archiveOpts, c.dumpJSON, al)
}

// doExparchive performs the exparchive operation for an isolate specified by archiveOpts.
// dumpJSON is the path to write a JSON summary of the uploaded isolate, in the same format as batch_archive.
func doExpArchive(ctx context.Context, client *isolatedclient.Client, archiveOpts *isolate.ArchiveOptions, dumpJSON string, al archiveLogger) error {
	// Set up a checker and uploader. We limit the uploader to one concurrent
	// upload, since the uploads are all coming from disk (with the exception of
	// the isolated JSON itself) and we only want a single goroutine reading from
	// disk at once.
	checker := NewChecker(ctx, client)
	uploader := NewUploader(ctx, client, 1)
	archiver := NewTarringArchiver(checker, uploader)

	isolSummary, err := archiver.Archive(archiveOpts)
	if err != nil {
		return err
	}

	// Make sure that all pending items have been checked.
	if err := checker.Close(); err != nil {
		return err
	}

	// Make sure that all the uploads have completed successfully.
	if err := uploader.Close(); err != nil {
		return err
	}

	printSummary(al, isolSummary)
	if dumpJSON != "" {
		f, err := os.OpenFile(dumpJSON, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return err
		}
		writeSummaryJSON(f, isolSummary)
		f.Close()
	}

	al.LogSummary(ctx, int64(checker.Hit.Count), int64(checker.Miss.Count), units.Size(checker.Hit.Bytes), units.Size(checker.Miss.Bytes), []string{string(isolSummary.Digest)})
	return nil
}

func writeSummaryJSON(w io.Writer, summaries ...IsolatedSummary) error {
	m := make(map[string]isolated.HexDigest)
	for _, summary := range summaries {
		m[summary.Name] = summary.Digest
	}

	return json.NewEncoder(w).Encode(m)
}

func printSummary(al archiveLogger, summary IsolatedSummary) {
	al.Printf("%s\t%s\n", summary.Digest, summary.Name)
}

func (c *expArchiveRun) parseFlags(args []string) error {
	if len(args) != 0 {
		return errors.New("position arguments not expected")
	}
	if err := c.commonServerFlags.Parse(); err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := c.isolateFlags.Parse(cwd, RequireIsolateFile&RequireIsolatedFile); err != nil {
		return err
	}
	return nil
}

func (c *expArchiveRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	fmt.Fprintln(a.GetErr(), "WARNING: this command is experimental")
	if err := c.parseFlags(args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	if err := c.main(); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}
