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
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/maruel/subcommands"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/rts/filegraph/git"
)

func cmdSelect() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `select`,
		ShortDesc: "select tests to run",
		LongDesc: text.Doc(`
			Select tests to run.

			The stdin must be a JSON object described by selectInput struct.
			The stdout is a JSON object described by selectOutput struct.
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
				and computed by "rts-chromium create-model".
			`))
			r.Flags.Float64Var(&r.targetChangeRecall, "target-change-recall", 0.99, text.Doc(`
				The target fraction of bad changes to be caught by the selection strategy.
				A value in (0.0, 1.0).
			`))
			return r
		},
	}
}

type selectRun struct {
	baseCommandRun
	changedFilesPath   string
	modelDir           string
	targetChangeRecall float64

	threhsold         threhsold
	changedFiles      stringset.Set
	existingTestFiles stringset.Set
	fg                *git.Graph
}

func (r *selectRun) validateFlags() error {
	switch {
	case r.changedFilesPath == "":
		return errors.New("-changed-files is required")
	case r.modelDir == "":
		return errors.New("-model-dir is required")
	case r.targetChangeRecall <= 0 || r.targetChangeRecall >= 1.0:
		return errors.New("-target-change-recall must be in (0.0, 1.0)")
	case len(flag.Args()) > 0:
		return errors.New("unexpected positional arguments")
	}
	return nil
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

	return nil
}

func (r *selectRun) selectTests(excludedTestFiles io.Writer) (err error) {
	return queryFileGraph(r.fg, r.changedFiles.ToSlice(), func(fd fileDistance) error {
		if fd.Rank <= r.threhsold.MaxRank || fd.Distance <= r.threhsold.MaxDistance {
			// This file too close to excludei t.
			return nil
		}

		fileName := fd.Node.Name()
		if !r.existingTestFiles.Has(fileName) {
			// This is not a test file.
			return nil
		}

		_, err := fmt.Fprintln(excludedTestFiles, fileName)
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
		err := r.loadExistingTestFiles()
		return errors.Annotate(err, "failed to load existing test files").Err()
	})

	eg.Go(func() error {
		err := r.loadChangedFiles()
		return errors.Annotate(err, "failed to load changed files").Err()
	})

	eg.Go(func() error {
		err := r.selectThrehsold()
		return errors.Annotate(err, "failed to select threshold").Err()
	})

	return eg.Wait()
}

// loadGraph loads the file grpah from the model.
func (r *selectRun) loadGraph() error {
	fileName := filepath.Join(r.modelDir, "git-file-graph", "graph.bin")
	f, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer f.Close()

	return r.fg.Read(bufio.NewReader(f))
}

// loadExistingTestFiles loads the list of existing test files from the model.
func (r *selectRun) loadExistingTestFiles() error {
	fileName := filepath.Join(r.modelDir, "existing-test-files.txt")
	var err error
	r.existingTestFiles, err = loadStringSet(fileName)
	return err
}

// loadChangedFiles loads the list of changed files based on -changed-files
// flag.
func (r *selectRun) loadChangedFiles() error {
	var err error
	r.changedFiles, err = loadStringSet(r.changedFilesPath)
	return err
}

// selectThreshold initializes r.threhsold based on r.targetChangeRecall.
func (r *selectRun) selectThrehsold() error {
	// Read the threshold file.
	fileName := filepath.Join(r.modelDir, "git-file-graph", "thresholds.json")
	fileBytes, err := ioutil.ReadFile(fileName)
	if err != nil {
		return err
	}
	var parsed threhsoldFile
	if err := json.Unmarshal(fileBytes, &parsed); err != nil {
		return errors.Annotate(err, "failed to parse the file").Err()
	}

	// Choose the threshold.
	const notChosenYet = 54 // sentinel value
	r.threhsold.ChangeRecall = notChosenYet
	for _, t := range parsed.Threhsolds {
		if t.ChangeRecall >= r.targetChangeRecall && r.threhsold.ChangeRecall > t.ChangeRecall {
			r.threhsold = *t
		}
	}
	if r.threhsold.ChangeRecall == notChosenYet {
		return errors.Annotate(err, "failed to choose the threshold for target change recall %.2f; the threhsold file must be corrupted").Err()
	}

	return nil
}

func loadStringSet(fileName string) (stringset.Set, error) {
	f, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scan := bufio.NewScanner(f)
	set := stringset.New(0)
	for scan.Scan() {
		set.Add(scan.Text())
	}
	return set, scan.Err()
}
