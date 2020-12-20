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

package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/maruel/subcommands"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/rts"
	"go.chromium.org/luci/rts/filegraph/git"
)

func cmdSelect() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `select -changed-files <path> -model-dir <path> -excluded-test-files <path>`,
		ShortDesc: "select tests to run",
		LongDesc: text.Doc(`
			Select tests to run.

			Flags -changed-files, -model-dir and -excluded-test-files are required.
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &selectRun{}
			r.Flags.StringVar(&r.changedFilesPath, "changed-files", "", text.Doc(`
				Path to the file with changed files.
				Each line of the file must be a filename, with "//" prefix.
			`))
			r.Flags.StringVar(&r.modelDir, "model-dir", "", text.Doc(`
				Path to the directory with the model files.
				Normally it is coming from CIPD package "chromium/rts/model"
				and computed by "rts-chromium create-model" command.
			`))
			r.Flags.StringVar(&r.skipTestFilesPath, "skip-test-files", "", text.Doc(`
				Path to the file where to write test file names that should be skipped.
				Each line of the file will be a filename, with "//" prefix.
			`))
			r.Flags.Float64Var(&r.targetChangeRecall, "target-change-recall", 0.99, text.Doc(`
				The target fraction of bad changes to be caught by the selection strategy.
				It must be a value in (0.0, 1.0) range.
			`))
			return r
		},
	}
}

type selectRun struct {
	baseCommandRun

	// Direct input.

	changedFilesPath   string
	modelDir           string
	skipTestFilesPath  string
	targetChangeRecall float64

	// Indirect input.

	testFiles    stringset.Set
	threshold    *rts.Affectedness
	changedFiles stringset.Set
	gitModel     git.SelectionModel
}

func (r *selectRun) validateFlags() error {
	switch {
	case r.changedFilesPath == "":
		return errors.New("-changed-files is required")
	case r.modelDir == "":
		return errors.New("-model-dir is required")
	case r.skipTestFilesPath == "":
		return errors.New("-skip-test-files is required")
	case r.targetChangeRecall <= 0 || r.targetChangeRecall >= 1.0:
		return errors.New("-target-change-recall must be in (0.0, 1.0)")
	case len(flag.Args()) > 0:
		return errors.New("unexpected positional arguments")
	default:
		return nil
	}
}

func (r *selectRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	return r.done(r.run(ctx))
}

func (r *selectRun) run(ctx context.Context) error {
	if err := r.validateFlags(); err != nil {
		return err
	}

	if err := r.loadInput(ctx); err != nil {
		return err
	}

	f, err := os.Create(r.skipTestFilesPath)
	if err != nil {
		return errors.Annotate(err, "failed to create skip file at %q", r.skipTestFilesPath).Err()
	}
	defer f.Close()
	return r.selectTests(func(skippedFile string) error {
		_, err := fmt.Fprintln(f, skippedFile)
		return err
	})
}

// loadInput loads all the input.
func (r *selectRun) loadInput(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()

	eg.Go(func() error {
		err := r.loadGraph()
		return errors.Annotate(err, "failed to load file graph").Err()
	})

	eg.Go(func() error {
		err := r.loadTestFileSet()
		return errors.Annotate(err, "failed to load test files list").Err()
	})

	eg.Go(func() error {
		err := r.loadChangedFileSet()
		return errors.Annotate(err, "failed to load changed files set").Err()
	})

	eg.Go(func() error {
		err := r.selectThreshold()
		return errors.Annotate(err, "failed to select threshold").Err()
	})

	return eg.Wait()
}

// loadGraph loads the file graph from the model.
func (r *selectRun) loadGraph() error {
	fileName := filepath.Join(r.modelDir, "git-file-graph", "graph.fg")
	f, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer f.Close()

	r.gitModel.Graph = &git.Graph{}
	return r.gitModel.Graph.Read(bufio.NewReader(f))
}

// loadTestFileSet loads the set of existing test files from the model.
func (r *selectRun) loadTestFileSet() error {
	fileName := filepath.Join(r.modelDir, "test-file-set.txt")
	var err error
	r.testFiles, err = loadStringSet(fileName)
	return err
}

// loadChangedFileSet loads the set of changed files based on -changed-files
// flag.
func (r *selectRun) loadChangedFileSet() error {
	var err error
	r.changedFiles, err = loadStringSet(r.changedFilesPath)
	return err
}

// selectThreshold initializes r.threshold based on r.targetChangeRecall.
func (r *selectRun) selectThreshold() error {
	// // Read the threshold file.
	// fileName := filepath.Join(r.modelDir, "git-file-graph", "thresholds.json")
	// fileBytes, err := ioutil.ReadFile(fileName)
	// if err != nil {
	// 	return err
	// }
	// var parsed thresholdFile
	// if err := json.Unmarshal(fileBytes, &parsed); err != nil {
	// 	return errors.Annotate(err, "failed to parse the file").Err()
	// }

	// // Choose the threshold.
	// const notChosenYet = 54 // sentinel value
	// r.threshold.ChangeRecall = notChosenYet
	// for _, t := range parsed.Thresholds {
	// 	if t.ChangeRecall >= r.targetChangeRecall && r.threshold.ChangeRecall > t.ChangeRecall {
	// 		r.threshold = *t
	// 	}
	// }
	// if r.threshold.ChangeRecall == notChosenYet {
	// 	return errors.Annotate(
	// 		err,
	// 		"failed to choose the threshold for target change recall %.2f; the threshold file must be corrupted",
	// 		r.targetChangeRecall,
	// 	).Err()
	// }

	// TODO(nodir): choose the threshold based on target change recall.
	r.threshold = &rts.Affectedness{Distance: 14, Rank: 100000}
	return nil
}

// loadStringSet loads a set of newline-separated strings from a text file.
func loadStringSet(fileName string) (stringset.Set, error) {
	f, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	set := stringset.New(0)
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		set.Add(scan.Text())
	}
	return set, scan.Err()
}
