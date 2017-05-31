// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/client/isolate"
	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/eventlog"
	logpb "github.com/luci/luci-go/common/eventlog/proto"
	"github.com/luci/luci-go/common/isolated"
	"github.com/luci/luci-go/common/isolatedclient"
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
	dumpJSON          string
}

// Item represents a file or symlink referenced by an isolate file.
type Item struct {
	Path    string
	RelPath string
	Size    int64
	Mode    os.FileMode

	Digest isolated.HexDigest
}

// main contains the core logic for experimental archive.
func (c *expArchiveRun) main() error {
	// TODO(djd): This func is long and has a lot of internal complexity (like,
	// such as, archiveCallback). Refactor.

	start := time.Now()
	archiveOpts := &c.isolateFlags.ArchiveOptions
	// Parse the incoming isolate file.
	deps, rootDir, isol, err := isolate.ProcessIsolate(archiveOpts)
	if err != nil {
		return fmt.Errorf("failed to process isolate: %v", err)
	}
	log.Printf("Isolate referenced %d deps", len(deps))

	// Set up a background context which is cancelled when this function returns.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create the isolated client which connects to the isolate server.
	authCl, err := c.createAuthClient()
	if err != nil {
		return err
	}
	client := isolatedclient.New(nil, authCl, c.isolatedFlags.ServerURL, c.isolatedFlags.Namespace, nil, nil)

	// Set up a checker and uploader. We limit the uploader to one concurrent
	// upload, since the uploads are all coming from disk (with the exception of
	// the isolated JSON itself) and we only want a single goroutine reading from
	// disk at once.
	checker := NewChecker(ctx, client)
	uploader := NewUploader(ctx, client, 1)

	// Walk each of the deps, partioning the results into symlinks and files categorised by size.
	var links, archiveFiles, indivFiles []*Item
	var archiveSize, indivSize int64 // Cumulative size of archived/individual files.
	for _, dep := range deps {
		// Try to walk dep. If dep is a file (or symlink), the inner function is called exactly once.
		err := filepath.Walk(filepath.Clean(dep), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			relPath, err := filepath.Rel(rootDir, path)
			if err != nil {
				return err
			}

			item := &Item{
				Path:    path,
				RelPath: relPath,
				Mode:    info.Mode(),
				Size:    info.Size(),
			}

			switch {
			case item.Mode&os.ModeSymlink == os.ModeSymlink:
				links = append(links, item)
			case item.Size < archiveThreshold:
				archiveFiles = append(archiveFiles, item)
				archiveSize += item.Size
			default:
				indivFiles = append(indivFiles, item)
				indivSize += item.Size
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	// Construct a map of the files that constitute the isolate.
	files := make(map[string]isolated.File)

	log.Printf("Isolate expanded to %d files (total size %s) and %d symlinks", len(archiveFiles)+len(indivFiles), humanize.Bytes(uint64(archiveSize+indivSize)), len(links))
	log.Printf("\t%d files (%s) to be isolated individually", len(indivFiles), humanize.Bytes(uint64(indivSize)))
	log.Printf("\t%d files (%s) to be isolated in archives", len(archiveFiles), humanize.Bytes(uint64(archiveSize)))

	// Handle the symlinks.
	for _, item := range links {
		l, err := os.Readlink(item.Path)
		if err != nil {
			return fmt.Errorf("unable to resolve symlink for %q: %v", item.Path, err)
		}
		files[item.RelPath] = isolated.SymLink(l)
	}

	// Handle the small to-be-archived files.
	bundles := ShardItems(archiveFiles, archiveMaxSize)
	log.Printf("\t%d TAR archives to be isolated", len(bundles))

	for _, bundle := range bundles {
		bundle := bundle
		digest, tarSize, err := bundle.Digest()
		if err != nil {
			return err
		}

		log.Printf("Created tar archive %q (%s)", digest, humanize.Bytes(uint64(tarSize)))
		log.Printf("\tcontains %d files (total %s)", len(bundle.Items), humanize.Bytes(uint64(bundle.ItemSize)))
		// Mint an item for this tar.
		item := &Item{
			Path:    fmt.Sprintf(".%s.tar", digest),
			RelPath: fmt.Sprintf(".%s.tar", digest),
			Size:    tarSize,
			Mode:    0644, // Read
			Digest:  digest,
		}
		files[item.RelPath] = isolated.TarFile(item.Digest, int(item.Mode), item.Size)

		checker.AddItem(item, false, func(item *Item, ps *isolatedclient.PushState) {
			if ps == nil {
				return
			}
			log.Printf("QUEUED %q for upload", item.RelPath)
			uploader.Upload(item.RelPath, bundle.Contents, ps, func() {
				log.Printf("UPLOADED %q", item.RelPath)
			})
		})
	}

	// Handle the large individually-uploaded files.
	for _, item := range indivFiles {
		d, err := hashFile(item.Path)
		if err != nil {
			return err
		}
		item.Digest = d
		files[item.RelPath] = isolated.BasicFile(item.Digest, int(item.Mode), item.Size)
		checker.AddItem(item, false, func(item *Item, ps *isolatedclient.PushState) {
			if ps == nil {
				return
			}
			log.Printf("QUEUED %q for upload", item.RelPath)
			uploader.UploadFile(item, ps, func() {
				log.Printf("UPLOADED %q", item.RelPath)
			})
		})
	}

	// Marshal the isolated file into JSON, and create an Item to describe it.
	isol.Files = files
	isolJSON, err := json.Marshal(isol)
	if err != nil {
		return err
	}
	isolItem := &Item{
		Path:    archiveOpts.Isolated,
		RelPath: filepath.Base(archiveOpts.Isolated),
		Digest:  isolated.HashBytes(isolJSON),
		Size:    int64(len(isolJSON)),
	}

	// Check and upload isolate JSON.
	checker.AddItem(isolItem, true, func(item *Item, ps *isolatedclient.PushState) {
		if ps == nil {
			return
		}
		log.Printf("QUEUED %q for upload", item.RelPath)
		uploader.UploadBytes(item.RelPath, isolJSON, ps, func() {
			log.Printf("UPLOADED %q", item.RelPath)
		})
	})

	// Make sure that all pending items have been checked.
	if err := checker.Close(); err != nil {
		return err
	}

	// Make sure that all the uploads have completed successfully.
	if err := uploader.Close(); err != nil {
		return err
	}

	// Write the isolated file, and emit its digest to stdout.
	if err := ioutil.WriteFile(archiveOpts.Isolated, isolJSON, 0644); err != nil {
		return err
	}
	fmt.Printf("%s\t%s\n", isolItem.Digest, filepath.Base(archiveOpts.Isolated))

	// Optionally, write the digest of the isolated file as JSON (in the same
	// format as batch_archive).
	if c.dumpJSON != "" {
		// The name is the base name of the isolated file, extension stripped.
		name := filepath.Base(archiveOpts.Isolated)
		if i := strings.LastIndex(name, "."); i != -1 {
			name = name[:i]
		}
		j, err := json.Marshal(map[string]isolated.HexDigest{
			name: isolItem.Digest,
		})
		if err != nil {
			return err
		}
		if err := ioutil.WriteFile(c.dumpJSON, j, 0644); err != nil {
			return err
		}
	}

	end := time.Now()

	if endpoint := eventlogEndpoint(c.isolateFlags.EventlogEndpoint); endpoint != "" {
		logger := eventlog.NewClient(ctx, endpoint)

		// TODO(mcgreevy): fill out more stats in archiveDetails.
		archiveDetails := &logpb.IsolateClientEvent_ArchiveDetails{
			HitCount:    proto.Int64(int64(checker.Hit.Count)),
			MissCount:   proto.Int64(int64(checker.Miss.Count)),
			HitBytes:    &checker.Hit.Bytes,
			MissBytes:   &checker.Miss.Bytes,
			IsolateHash: []string{string(isolItem.Digest)},
		}
		if err := logStats(ctx, logger, start, end, archiveDetails, getBuildbotInfo()); err != nil {
			log.Printf("Failed to log to eventlog: %v", err)
		}
	}

	return nil
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
	if len(c.isolateFlags.ArchiveOptions.Blacklist) != 0 {
		fmt.Fprintf(a.GetErr(), "%s: blacklist is not supported\n", a.GetName())
		return 1
	}
	if err := c.main(); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}

func hashFile(path string) (isolated.HexDigest, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return isolated.Hash(f)
}

// buildbotInfo contains information about the build in which this command was run.
type buildbotInfo struct {
	// Variables which are not present in the environment are nil.
	master, builder, buildID, slave *string
}

// getBuildbotInfo poulates a buildbotInfo with information from the environment.
func getBuildbotInfo() *buildbotInfo {
	getEnvValue := func(key string) *string {
		if val, ok := os.LookupEnv(key); ok {
			return &val
		}
		return nil
	}

	return &buildbotInfo{
		master:  getEnvValue("BUILDBOT_MASTERNAME"),
		builder: getEnvValue("BUILDBOT_BUILDERNAME"),
		buildID: getEnvValue("BUILDBOT_BUILDNUMBER"),
		slave:   getEnvValue("BUILDBOT_SLAVENAME"),
	}
}

func logStats(ctx context.Context, logger *eventlog.Client, start, end time.Time, archiveDetails *logpb.IsolateClientEvent_ArchiveDetails, bi *buildbotInfo) error {
	event := logger.NewLogEvent(ctx, eventlog.Point())
	event.InfraEvent.IsolateClientEvent = &logpb.IsolateClientEvent{
		Binary: &logpb.Binary{
			Name:          proto.String("isolate"),
			VersionNumber: proto.String(version),
		},
		Operation:      logpb.IsolateClientEvent_ARCHIVE.Enum(),
		ArchiveDetails: archiveDetails,
		Master:         bi.master,
		Builder:        bi.builder,
		BuildId:        bi.buildID,
		Slave:          bi.slave,
		StartTsUsec:    proto.Int64(int64(start.UnixNano() / 1e3)),
		EndTsUsec:      proto.Int64(int64(end.UnixNano() / 1e3)),
	}
	return logger.LogSync(ctx, event)
}

func eventlogEndpoint(endpointFlag string) string {
	switch endpointFlag {
	case "test":
		return eventlog.TestEndpoint
	case "prod":
		return eventlog.ProdEndpoint
	default:
		return endpointFlag
	}
}
