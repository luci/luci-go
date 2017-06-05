// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"

	"github.com/luci/luci-go/client/archiver"
	"github.com/luci/luci-go/client/internal/common"
	"github.com/luci/luci-go/client/isolate"
	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/data/text/units"
	logpb "github.com/luci/luci-go/common/eventlog/proto"
	"github.com/luci/luci-go/common/isolatedclient"
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
			return &c
		},
	}
}

type archiveRun struct {
	commonServerFlags
	isolateFlags
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
	out := os.Stdout
	prefix := "\n"
	if c.defaultFlags.Quiet {
		prefix = ""
	}
	start := time.Now()
	client, err := c.createAuthClient()
	if err != nil {
		return err
	}
	ctx := c.defaultFlags.MakeLoggingContext(os.Stderr)
	arch := archiver.New(ctx, isolatedclient.New(nil, client, c.isolatedFlags.ServerURL, c.isolatedFlags.Namespace, nil, nil), out)
	common.CancelOnCtrlC(arch)
	item := isolate.Archive(arch, &c.ArchiveOptions)
	item.WaitForHashed()
	if err = item.Error(); err != nil {
		fmt.Printf("%s%s  %s\n", prefix, filepath.Base(c.Isolate), err)
	} else {
		fmt.Printf("%s%s  %s\n", prefix, item.Digest(), filepath.Base(c.Isolate))
	}
	if err2 := arch.Close(); err == nil {
		err = err2
	}
	stats := arch.Stats()
	if !c.defaultFlags.Quiet {
		duration := time.Since(start)
		fmt.Fprintf(os.Stderr, "Hits    : %5d (%s)\n", stats.TotalHits(), stats.TotalBytesHits())
		fmt.Fprintf(os.Stderr, "Misses  : %5d (%s)\n", stats.TotalMisses(), stats.TotalBytesPushed())
		fmt.Fprintf(os.Stderr, "Duration: %s\n", units.Round(duration, time.Millisecond))
	}

	end := time.Now()

	archiveDetails := &logpb.IsolateClientEvent_ArchiveDetails{
		HitCount:  proto.Int64(int64(stats.TotalHits())),
		MissCount: proto.Int64(int64(stats.TotalMisses())),
		HitBytes:  proto.Int64(int64(stats.TotalBytesHits())),
		MissBytes: proto.Int64(int64(stats.TotalBytesPushed())),
	}
	if item.Error() != nil {
		archiveDetails.IsolateHash = []string{string(item.Digest())}
	}
	eventlogger := NewLogger(ctx, c.isolateFlags.EventlogEndpoint)
	op := logpb.IsolateClientEvent_LEGACY_ARCHIVE.Enum()
	if err := eventlogger.logStats(ctx, op, start, end, archiveDetails); err != nil {
		log.Printf("Failed to log to eventlog: %v", err)
	}
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
