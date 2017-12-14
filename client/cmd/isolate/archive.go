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
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/archiver"
	"go.chromium.org/luci/client/isolate"
	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/data/text/units"
	logpb "go.chromium.org/luci/common/eventlog/proto"
	"go.chromium.org/luci/common/isolatedclient"
)

func cmdArchive(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "archive <options>",
		ShortDesc: "creates a .isolated file and uploads the tree to an isolate server.",
		LongDesc:  "All the files listed in the .isolated file are put in the isolate server cache",
		CommandRun: func() subcommands.CommandRun {
			c := archiveRun{}
			c.commonServerFlags.Init(defaultAuthOpts)
			c.isolateFlags.Init(&c.Flags)
			c.loggingFlags.Init(&c.Flags)
			c.Flags.BoolVar(&c.expArchive, "exparchive", false, "Whether to use the new exparchive implementation, which tars small files before uploading them.")
			c.Flags.IntVar(&c.maxConcurrentUploads, "max-concurrent-uploads", 1, "The maxiumum number of in-flight uploads.")
			c.Flags.StringVar(&c.dumpJSON, "dump-json", "",
				"Write isolated digests of archived trees to this file as JSON")
			return &c
		},
	}
}

type archiveRun struct {
	commonServerFlags
	isolateFlags
	loggingFlags         loggingFlags
	expArchive           bool
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
	if err := c.isolateFlags.Parse(cwd, RequireIsolatedFile); err != nil {
		return err
	}
	if len(args) != 0 {
		return errors.New("position arguments not expected")
	}
	return nil
}

func (c *archiveRun) main(a subcommands.Application, args []string) error {
	start := time.Now()
	authCl, err := c.createAuthClient()
	if err != nil {
		return err
	}
	ctx := c.defaultFlags.MakeLoggingContext(os.Stderr)
	client := isolatedclient.New(nil, authCl, c.isolatedFlags.ServerURL, c.isolatedFlags.Namespace, nil, nil)

	al := archiveLogger{
		logger: NewLogger(ctx, c.loggingFlags.EventlogEndpoint),
		start:  start,
		quiet:  c.defaultFlags.Quiet,
	}

	if c.expArchive {
		al.operation = logpb.IsolateClientEvent_ARCHIVE.Enum()
		return doExpArchive(ctx, client, &c.ArchiveOptions, c.dumpJSON, c.maxConcurrentUploads, al)
	}
	al.operation = logpb.IsolateClientEvent_LEGACY_ARCHIVE.Enum()
	return doArchive(ctx, client, &c.ArchiveOptions, c.dumpJSON, al)
}

// archiveLogger reports stats to eventlog and stderr.
type archiveLogger struct {
	logger    *IsolateEventLogger
	operation *logpb.IsolateClientEvent_Operation
	start     time.Time
	quiet     bool
}

// LogSummary logs (to eventlog and stderr) a high-level summary of archive operations(s).
func (al *archiveLogger) LogSummary(ctx context.Context, hits, misses int64, bytesHit, bytesPushed units.Size, digests []string) {
	archiveDetails := &logpb.IsolateClientEvent_ArchiveDetails{
		HitCount:    proto.Int64(hits),
		MissCount:   proto.Int64(misses),
		HitBytes:    proto.Int64(int64(bytesHit)),
		MissBytes:   proto.Int64(int64(bytesPushed)),
		IsolateHash: digests,
	}

	end := time.Now()
	if err := al.logger.logStats(ctx, al.operation, al.start, end, archiveDetails); err != nil {
		log.Printf("Failed to log to eventlog: %v", err)
	}

	if !al.quiet {
		duration := end.Sub(al.start)
		fmt.Fprintf(os.Stderr, "Hits    : %5d (%s)\n", hits, bytesHit)
		fmt.Fprintf(os.Stderr, "Misses  : %5d (%s)\n", misses, bytesPushed)
		fmt.Fprintf(os.Stderr, "Duration: %s\n", units.Round(duration, time.Millisecond))
	}
}

// Print acts like fmt.Printf, but may prepend a prefix to format, depending on the value of al.quiet.
func (al *archiveLogger) Printf(format string, a ...interface{}) (n int, err error) {
	return al.Fprintf(os.Stdout, format, a...)
}

// Print acts like fmt.fprintf, but may prepend a prefix to format, depending on the value of al.quiet.
func (al *archiveLogger) Fprintf(w io.Writer, format string, a ...interface{}) (n int, err error) {
	prefix := "\n"
	if al.quiet {
		prefix = ""
	}
	args := []interface{}{prefix}
	args = append(args, a...)
	return fmt.Printf("%s"+format, args...)
}

// doArchive performs the archive operation for an isolate specified by archiveOpts.
func doArchive(ctx context.Context, client *isolatedclient.Client, archiveOpts *isolate.ArchiveOptions, dumpJSON string, al archiveLogger) error {
	arch := archiver.New(ctx, client, os.Stdout)
	CancelOnCtrlC(arch)
	item := isolate.Archive(arch, archiveOpts)
	item.WaitForHashed()
	var err error
	if err = item.Error(); err != nil {
		al.Printf("%s  %s\n", filepath.Base(archiveOpts.Isolate), err)
	} else {
		filename := filepath.Base(archiveOpts.Isolate)
		name := filename[:len(filename)-len(filepath.Ext(filename))]
		summary := IsolatedSummary{
			Name:   name,
			Digest: item.Digest(),
		}
		printSummary(al, summary)
		if err := dumpSummaryJSON(dumpJSON, summary); err != nil {
			al.Printf("failed to dump json: %v", err)
		}
	}
	if err2 := arch.Close(); err == nil {
		err = err2
	}

	stats := arch.Stats()

	var digests []string
	if item.Error() != nil {
		digests = []string{string(item.Digest())}
	}

	al.LogSummary(ctx, int64(stats.TotalHits()), int64(stats.TotalMisses()), stats.TotalBytesHits(), stats.TotalBytesPushed(), digests)
	return err
}

func (c *archiveRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
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

// CancelOnCtrlC is a temporary copy of the CancelOnCtrlC in internal/common/concurrent.go
// This is needed until the old archive and batcharchive code (which uses Cancelers) is removed.
// It operates on a concrete Archiver to avoid the dependency on Canceler.
func CancelOnCtrlC(arch *archiver.Archiver) {
	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt)
	go func() {
		defer signal.Stop(interrupted)
		select {
		case <-interrupted:
			arch.Cancel(errors.New("Ctrl-C"))
		case <-arch.Channel():
		}
	}()
}
