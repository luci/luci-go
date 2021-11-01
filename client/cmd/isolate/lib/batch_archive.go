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
	"io"
	"log"
	"os"
	"runtime/pprof"
	"runtime/trace"
	"strings"
	"time"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/client/archiver/tarring"
	"go.chromium.org/luci/client/casclient"
	"go.chromium.org/luci/client/isolate"
	"go.chromium.org/luci/common/data/text/units"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/signals"
)

// CmdBatchArchive returns an object for the `batcharchive` subcommand.
func CmdBatchArchive(defaultAuthOpts auth.Options) *subcommands.Command {
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
			c.casFlags.Init(&c.Flags)
			c.Flags.StringVar(&c.dumpJSON, "dump-json", "", "Write isolated digests of archived trees to this file as JSON")
			c.Flags.IntVar(&c.maxConcurrentChecks, "max-concurrent-checks", 1, "The maximum number of in-flight check requests.")
			c.Flags.IntVar(&c.maxConcurrentUploads, "max-concurrent-uploads", 8, "The maximum number of in-flight uploads.")
			return &c
		},
	}
}

type batchArchiveRun struct {
	commonServerFlags
	casFlags             casclient.Flags
	dumpJSON             string
	maxConcurrentChecks  int
	maxConcurrentUploads int
}

func (c *batchArchiveRun) Parse(a subcommands.Application, args []string) error {
	if err := c.commonServerFlags.Parse(); err != nil {
		return err
	}
	if err := c.casFlags.Parse(); err != nil {
		return err
	}
	if len(args) == 0 {
		return errors.Reason("at least one isolate file required").Err()
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
	base.Flags.StringVar(&i.Isolated, "isolated", "", ".isolated file to generate")
	base.Flags.StringVar(&i.Isolated, "s", "", "Alias for --isolated")
	i.Init(&base.Flags)
	if err := base.GetFlags().Parse(args); err != nil {
		return nil, err
	}
	if err := i.Parse(cwd); err != nil {
		return nil, err
	}
	if base.GetFlags().NArg() > 0 {
		return nil, errors.Reason("no positional arguments expected").Err()
	}
	i.PostProcess(cwd)
	return &i.ArchiveOptions, nil
}

// convertPyToGoArchiveCMDArgs converts kv-args from old python isolate into go variants.
// Essentially converts "--X key value" into "--X key=value".
func convertPyToGoArchiveCMDArgs(args []string) []string {
	kvars := map[string]bool{
		"--path-variable":   true,
		"--config-variable": true,
	}
	var newArgs []string
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
	ctx, cancel := context.WithCancel(c.defaultFlags.MakeLoggingContext(os.Stderr))
	defer cancel()
	defer signals.HandleInterrupt(func() {
		pprof.Lookup("goroutine").WriteTo(os.Stderr, 1)
		cancel()
	})()

	ctx, task := trace.NewTask(ctx, "batcharchive")
	defer task.End()

	opts, err := toArchiveOptions(args)
	if err != nil {
		return errors.Annotate(err, "failed to process input JSONs").Err()
	}

	al := &archiveLogger{
		start: start,
		quiet: c.defaultFlags.Quiet,
	}

	if c.casFlags.UseCAS() {
		ctx, err := casclient.ContextWithMetadata(ctx, "isolate")
		if err != nil {
			return err
		}
		_, err = c.uploadToCAS(ctx, c.dumpJSON, c.commonServerFlags.parsedAuthOpts, &c.casFlags, al, opts...)
		return err
	}

	return c.batchArchiveToIsolate(ctx, al, opts)
}

func toArchiveOptions(genJSONPaths []string) ([]*isolate.ArchiveOptions, error) {
	opts := make([]*isolate.ArchiveOptions, len(genJSONPaths))
	for i, genJSONPath := range genJSONPaths {
		o, err := processGenJSON(genJSONPath)
		if err != nil {
			return nil, errors.Annotate(err, "%q", genJSONPath).Err()
		}
		opts[i] = o
	}
	return opts, nil
}

// batchArchiveToIsolate archives a series of isolate files to isolate server.
func (c *batchArchiveRun) batchArchiveToIsolate(ctx context.Context, al *archiveLogger, opts []*isolate.ArchiveOptions) error {
	authClient, err := c.createAuthClient(ctx)
	if err != nil {
		return err
	}
	client, err := c.createIsolatedClient(authClient)
	if err != nil {
		return err
	}

	// Set up a checker and uploader. We limit the uploader to one concurrent
	// upload, since the uploads are all coming from disk (with the exception of
	// the isolated JSON itself) and we only want a single goroutine reading from
	// disk at once.
	checker := tarring.NewChecker(ctx, client, c.maxConcurrentChecks)
	uploader := tarring.NewUploader(ctx, client, c.maxConcurrentUploads)
	a := tarring.NewArchiver(checker, uploader)

	var errArchive error
	var isolSummaries []tarring.IsolatedSummary
	for _, opt := range opts {
		// Parse the incoming isolate file.
		deps, rootDir, isol, err := isolate.ProcessIsolate(opt)
		if err != nil {
			return errors.Annotate(err, "isolate %s: failed to process", opt.Isolate).Err()
		}
		log.Printf("Isolate %s referenced %d deps", opt.Isolate, len(deps))

		isolSummary, err := a.Archive(&tarring.ArchiveArgs{
			Deps:          deps,
			RootDir:       rootDir,
			IgnoredPathRe: opt.IgnoredPathFilterRe,
			Isolated:      opt.Isolated,
			Isol:          isol})
		if err != nil && errArchive == nil {
			errArchive = errors.Annotate(err, "isolate %s: failed to archive", opt.Isolate).Err()
		} else {
			al.printSummary(isolSummary)
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

	if err := dumpSummaryJSON(c.dumpJSON, isolSummaries...); err != nil {
		return err
	}

	al.LogSummary(ctx, checker.Hit.Count(), checker.Miss.Count(), units.Size(checker.Hit.Bytes()), units.Size(checker.Miss.Bytes()))
	return nil
}

// processGenJSON validates a genJSON file and returns the contents.
func processGenJSON(genJSONPath string) (*isolate.ArchiveOptions, error) {
	f, err := os.Open(genJSONPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return processGenJSONData(f)
}

// processGenJSONData implements processGenJSON, but operates on an io.Reader.
func processGenJSONData(r io.Reader) (*isolate.ArchiveOptions, error) {
	var data struct {
		Args    []string
		Dir     string
		Version int
	}
	if err := json.NewDecoder(r).Decode(&data); err != nil {
		return nil, errors.Annotate(err, "failed to decode").Err()
	}

	if data.Version != isolate.IsolatedGenJSONVersion {
		return nil, errors.Reason("unsupported version %d", data.Version).Err()
	}

	if fileInfo, err := os.Stat(data.Dir); err != nil || !fileInfo.IsDir() {
		return nil, errors.Reason("invalid dir %q", data.Dir).Err()
	}

	opts, err := parseArchiveCMD(data.Args, data.Dir)
	if err != nil {
		return nil, errors.Annotate(err, "invalid archive command").Err()
	}
	return opts, nil
}

func (c *batchArchiveRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	if err := c.Parse(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	defer c.profiler.Stop()
	if err := c.main(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), strings.Join(errors.RenderStack(err), "\n"))
		return 1
	}
	return 0
}
