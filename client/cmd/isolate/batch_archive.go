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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/archiver"
	"go.chromium.org/luci/client/isolate"
	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/data/text/units"
	logpb "go.chromium.org/luci/common/eventlog/proto"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
)

func cmdBatchArchive(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "batcharchive <options> file1 file2 ...",
		ShortDesc: "archives multiple isolated trees at once.",
		LongDesc: `Archives multiple isolated trees at once.

Using single command instead of multiple sequential invocations allows to cut
redundant work when isolated trees share common files (e.g. file hashes are
checked only once, their presence on the server is checked only once, and
so on).

Takes a list of paths to *.isolated.gen.json files that describe what trees to
isolate. Format of files is:
{
  "version": 1,
  "dir": <absolute path to a directory all other paths are relative to>,
  "args": [list of command line arguments for single 'archive' command]
}`,
		CommandRun: func() subcommands.CommandRun {
			c := batchArchiveRun{}
			c.commonServerFlags.Init(defaultAuthOpts)
			c.loggingFlags.Init(&c.Flags)
			c.Flags.StringVar(&c.dumpJSON, "dump-json", "", "Write isolated digests of archived trees to this file as JSON")
			c.Flags.BoolVar(&c.expArchive, "exparchive", false, "Whether to use the new exparchive implementation, which tars small files before uploading them.")
			return &c
		},
	}
}

type batchArchiveRun struct {
	commonServerFlags
	loggingFlags loggingFlags
	dumpJSON     string
	expArchive   bool
}

func (c *batchArchiveRun) Parse(a subcommands.Application, args []string) error {
	if err := c.commonServerFlags.Parse(); err != nil {
		return err
	}
	if len(args) == 0 {
		return errors.New("at least one isolate file required")
	}
	return nil
}

func parseArchiveCMD(args []string, cwd string) (*isolate.ArchiveOptions, error) {
	// Python isolate allows form "--XXXX-variable key value".
	// Golang flag pkg doesn't consider value to be part of --XXXX-variable flag.
	// Therefore, we convert all such "--XXXX-variable key value" to
	// "--XXXX-variable key --XXXX-variable value" form.
	// Note, that key doesn't have "=" in it in either case, but value might.
	// TODO(tandrii): eventually, we want to retire this hack.
	args = convertPyToGoArchiveCMDArgs(args)
	base := subcommands.CommandRunBase{}
	i := isolateFlags{}
	i.Init(&base.Flags)
	if err := base.GetFlags().Parse(args); err != nil {
		return nil, err
	}
	if err := i.Parse(cwd, RequireIsolatedFile); err != nil {
		return nil, err
	}
	if base.GetFlags().NArg() > 0 {
		return nil, fmt.Errorf("no positional arguments expected")
	}
	i.PostProcess(cwd)
	return &i.ArchiveOptions, nil
}

// convertPyToGoArchiveCMDArgs converts kv-args from old python isolate into go variants.
// Essentially converts "--X key value" into "--X key=value".
func convertPyToGoArchiveCMDArgs(args []string) []string {
	kvars := map[string]bool{
		"--path-variable": true, "--config-variable": true, "--extra-variable": true}
	newArgs := []string{}
	for i := 0; i < len(args); {
		newArgs = append(newArgs, args[i])
		kvar := args[i]
		i++
		if !kvars[kvar] {
			continue
		}
		if i >= len(args) {
			// Ignore unexpected behaviour, it'll be caught by flags.Parse() .
			break
		}
		appendArg := args[i]
		i++
		if !strings.Contains(appendArg, "=") && i < len(args) {
			// appendArg is key, and args[i] is value .
			appendArg = fmt.Sprintf("%s=%s", appendArg, args[i])
			i++
		}
		newArgs = append(newArgs, appendArg)
	}
	return newArgs
}

func (c *batchArchiveRun) main(a subcommands.Application, args []string) error {
	start := time.Now()
	ctx := c.defaultFlags.MakeLoggingContext(os.Stderr)

	authClient, err := c.createAuthClient()
	if err != nil {
		return err
	}
	client := isolatedclient.New(nil, authClient, c.isolatedFlags.ServerURL, c.isolatedFlags.Namespace, nil, nil)

	al := archiveLogger{
		logger:    NewLogger(ctx, c.loggingFlags.EventlogEndpoint),
		start:     start,
		quiet:     c.defaultFlags.Quiet,
		operation: logpb.IsolateClientEvent_BATCH_ARCHIVE.Enum(),
	}
	if c.expArchive {
		return doBatchExpArchive(ctx, client, al, c.dumpJSON, args)
	}
	return doBatchArchive(ctx, client, al, c.dumpJSON, args)
}

// doBatchArchive archives a series of isolates specified by genJSONPaths, using an archiver.Archiver.
func doBatchArchive(ctx context.Context, client *isolatedclient.Client, al archiveLogger, dumpJSONPath string, genJSONPaths []string) error {
	arch := archiver.New(ctx, client, os.Stdout)
	CancelOnCtrlC(arch)

	type namedItem struct {
		*archiver.Item
		name string
	}
	items := make(chan *namedItem, len(genJSONPaths))
	var wg sync.WaitGroup
	for _, genJSONPath := range genJSONPaths {
		wg.Add(1)
		go func(genJSONPath string) {
			defer wg.Done()
			if opts, err := processGenJSON(genJSONPath); err != nil {
				arch.Cancel(err)
			} else {
				items <- &namedItem{
					isolate.Archive(arch, opts),
					strippedIsolatedName(opts.Isolated),
				}
			}
		}(genJSONPath)
	}
	go func() {
		wg.Wait()
		close(items)
	}()

	data := map[string]isolated.HexDigest{}
	var digests []string
	for item := range items {
		item.WaitForHashed()
		if item.Error() == nil {
			d := item.Digest()
			data[item.name] = d
			digests = append(digests, string(d))
			al.Printf("%s  %s\n", d, item.name)
		} else {
			al.Fprintf(os.Stderr, "%s  %s\n", item.name, item.Error())
		}
	}
	err := arch.Close()
	// Only write the file once upload is confirmed.
	if err == nil && dumpJSONPath != "" {
		err = writeJSONDigestFile(dumpJSONPath, data)
	}

	stats := arch.Stats()
	al.LogSummary(ctx, int64(stats.TotalHits()), int64(stats.TotalMisses()), stats.TotalBytesHits(), stats.TotalBytesPushed(), digests)

	return err
}

// doBatchExpArchive archives a series of isolates specified by genJSONPaths, using the new exparchive codepath.
func doBatchExpArchive(ctx context.Context, client *isolatedclient.Client, al archiveLogger, dumpJSONPath string, genJSONPaths []string) error {
	// Set up a checker and uploader. We limit the uploader to one concurrent
	// upload, since the uploads are all coming from disk (with the exception of
	// the isolated JSON itself) and we only want a single goroutine reading from
	// disk at once.
	checker := NewChecker(ctx, client)
	uploader := NewUploader(ctx, client, 1)
	archiver := NewTarringArchiver(checker, uploader)

	var errArchive error
	var isolSummaries []IsolatedSummary
	for _, genJSONPath := range genJSONPaths {
		opts, err := processGenJSON(genJSONPath)
		if err != nil {
			return err
		}
		isolSummary, err := archiver.Archive(opts)
		if err != nil && errArchive == nil {
			errArchive = err
		} else {
			printSummary(al, isolSummary)
			isolSummaries = append(isolSummaries, isolSummary)
		}
	}
	if errArchive != nil {
		return errArchive
	}
	// Make sure that all pending items have been checked.
	if err := checker.Close(); err != nil {
		return err
	}

	// Make sure that all the uploads have completed successfully.
	if err := uploader.Close(); err != nil {
		return err
	}

	if err := dumpSummaryJSON(dumpJSONPath, isolSummaries...); err != nil {
		return err
	}

	al.LogSummary(ctx, int64(checker.Hit.Count), int64(checker.Miss.Count), units.Size(checker.Hit.Bytes), units.Size(checker.Miss.Bytes), digests(isolSummaries))
	return nil
}

// digests extracts the digests from the supplied IsolatedSummarys.
func digests(summaries []IsolatedSummary) []string {
	var result []string
	for _, summary := range summaries {
		result = append(result, string(summary.Digest))
	}
	return result
}

// processGenJSON validates a genJSON file and returns the contents.
func processGenJSON(genJSONPath string) (*isolate.ArchiveOptions, error) {
	f, err := os.Open(genJSONPath)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %s", genJSONPath, err)
	}
	defer f.Close()

	opts, err := processGenJSONData(f)
	if err != nil {
		return nil, fmt.Errorf("processing %s: %s", genJSONPath, err)
	}
	return opts, nil
}

// processGenJSONData performs the function of processGenJSON, but operates on an io.Reader.
func processGenJSONData(r io.Reader) (*isolate.ArchiveOptions, error) {
	data := &struct {
		Args    []string
		Dir     string
		Version int
	}{}
	if err := json.NewDecoder(r).Decode(data); err != nil {
		return nil, fmt.Errorf("failed to decode: %s", err)
	}

	if data.Version != isolate.IsolatedGenJSONVersion {
		return nil, fmt.Errorf("invalid version %d", data.Version)
	}

	if fileInfo, err := os.Stat(data.Dir); err != nil || !fileInfo.IsDir() {
		return nil, fmt.Errorf("invalid dir %s", data.Dir)
	}

	opts, err := parseArchiveCMD(data.Args, data.Dir)
	if err != nil {
		return nil, fmt.Errorf("invalid archive command: %s", err)
	}
	return opts, nil
}

// strippedIsolatedName returns the base name of an isolated path, with the extension (if any) removed.
func strippedIsolatedName(isolated string) string {
	name := filepath.Base(isolated)
	// Strip the extension if there is one.
	if dotIndex := strings.LastIndex(name, "."); dotIndex != -1 {
		return name[0:dotIndex]
	}
	return name
}

func writeJSONDigestFile(filePath string, data map[string]isolated.HexDigest) error {
	digestBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding digest JSON: %s", err)
	}
	return writeFile(filePath, digestBytes)
}

// writeFile writes data to filePath. File permission is set to user only.
func writeFile(filePath string, data []byte) error {
	f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("opening %s: %s", filePath, err)
	}
	// NOTE: We don't defer f.Close here, because it may return an error.

	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		return fmt.Errorf("writing %s: %s", filePath, writeErr)
	} else if closeErr != nil {
		return fmt.Errorf("closing %s: %s", filePath, closeErr)
	}
	return nil
}

func (c *batchArchiveRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
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
