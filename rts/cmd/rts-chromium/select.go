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
	"fmt"
	"os"
	"path/filepath"

	"github.com/maruel/subcommands"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/rts/filegraph/git"
	"go.chromium.org/luci/rts/presubmit/eval"
)

func cmdSelect() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `select -changed-files <path> -model-dir <path> -skip-test-files <path>`,
		ShortDesc: "compute the set of test files to skip",
		LongDesc: text.Doc(`
			Compute the set of test files to skip.

			Flags -changed-files, -model-dir and -skip-test-files are required.
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
				and precomputed by "rts-chromium create-model" command.
			`))
			r.Flags.StringVar(&r.skipTestFilesPath, "skip-test-files", "", text.Doc(`
				Path to the file where to write the test file names that should be skipped.
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
	changedFiles stringset.Set
	strategy     git.SelectionStrategy
	evalResult   *eval.Result
}

func (r *selectRun) validateFlags() error {
	switch {
	case r.changedFilesPath == "":
		return errors.New("-changed-files is required")
	case r.modelDir == "":
		return errors.New("-model-dir is required")
	case r.skipTestFilesPath == "":
		return errors.New("-skip-test-files is required")
	case !(r.targetChangeRecall > 0 && r.targetChangeRecall < 1):
		return errors.New("-target-change-recall must be in (0.0, 1.0) range")
	default:
		return nil
	}
}

func (r *selectRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	if len(args) != 0 {
		return r.done(errors.New("unexpected positional arguments"))
	}

	if err := r.validateFlags(); err != nil {
		return r.done(err)
	}

	if err := r.loadInput(ctx); err != nil {
		return r.done(err)
	}

	if t := r.chooseThreshold(); t != nil {
		return r.done(errors.Reason("no threshold for target change recall %.4f", r.targetChangeRecall).Err())
	} else {
		r.strategy.Threshold = t.Value
		logging.Infof(ctx, "chosen threhsold: %#v", r.strategy.Threshold)
	}

	f, err := os.Create(r.skipTestFilesPath)
	if err != nil {
		return r.done(errors.Annotate(err, "failed to create the skip file at %q", r.skipTestFilesPath).Err())
	}
	defer f.Close()
	return r.done(r.selectTests(func(skippedFile string) error {
		_, err := fmt.Fprintln(f, skippedFile)
		return err
	}))
}

// chooseThreshold returns the affectedness threshold based on
// r.targetChangeRecall and r.evalResult.
func (r *selectRun) chooseThreshold() *eval.Threshold {
	var ret *eval.Threshold
	for _, t := range r.evalResult.Thresholds {
		if t.ChangeRecall < r.targetChangeRecall {
			continue
		}
		if ret == nil || ret.ChangeRecall > t.ChangeRecall {
			ret = t
		}
	}
	return ret
}

// loadInput loads all the input of the subcommand.
func (r *selectRun) loadInput(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()

	gitGraphDir := filepath.Join(r.modelDir, "git-file-graph")
	eg.Go(func() error {
		err := r.loadGraph(filepath.Join(gitGraphDir, "graph.fg"))
		return errors.Annotate(err, "failed to load file graph").Err()
	})
	eg.Go(func() error {
		err := r.loadEvalResult(filepath.Join(gitGraphDir, "eval-result.json"))
		return errors.Annotate(err, "failed to load eval results").Err()
	})

	eg.Go(func() (err error) {
		r.testFiles, err = loadStringSet(filepath.Join(r.modelDir, "test-file-set.txt"))
		return errors.Annotate(err, "failed to load test files set").Err()
	})

	eg.Go(func() (err error) {
		r.changedFiles, err = loadStringSet(r.changedFilesPath)
		return errors.Annotate(err, "failed to load changed files set").Err()
	})

	return eg.Wait()
}

// loadEvalResult loads r.evalResult, including threhsolds.
func (r *selectRun) loadEvalResult(fileName string) error {
	f, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer f.Close()

	// TODO(nodir): consider turning eval.Result into a protobuf message for
	// encoding stability.
	return json.NewDecoder(f).Decode(r.evalResult)
}

// loadGraph loads r.strategy.Graph from the model.
func (r *selectRun) loadGraph(fileName string) error {
	f, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer f.Close()

	// Note: it might be dangerous to sync with the current checkout.
	// There might have been such change in the repo that the chosen threshold,
	// the model or both are no longer good. Thus, do not sync.
	r.strategy.Graph = &git.Graph{}
	return r.strategy.Graph.Read(bufio.NewReader(f))
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
