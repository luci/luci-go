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

	"cloud.google.com/go/bigquery"
	"github.com/maruel/subcommands"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/rts/filegraph/git"
	"go.chromium.org/luci/rts/presubmit/eval"
)

func cmdCreateModel(authOpt *auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `create-model -model-dir <path>`,
		ShortDesc: "create a model to be used by select subcommand",
		LongDesc:  "Create a model to be used by select subcommand",
		CommandRun: func() subcommands.CommandRun {
			r := &createModelRun{authOpt: authOpt}
			r.Flags.StringVar(&r.modelDir, "model-dir", "", text.Doc(`
				Path to the directory where to write the model files.
				The directory will be created if it does not exist.
			`))

			r.Flags.StringVar(&r.checkout, "checkout", "", "Path to a src.git checkout")
			r.Flags.IntVar(&r.loadOptions.MaxCommitSize, "fg-max-commit-size", 100, text.Doc(`
				Maximum number of files touched by a commit.
				Commits that exceed this limit are ignored.
				The rationale is that large commits provide a weak signal of file
				relatedness and are expensive to process, O(N^2).
			`))

			r.ev.RegisterFlags(&r.Flags)
			return r
		},
	}
}

type createModelRun struct {
	baseCommandRun
	modelDir string

	checkout    string
	loadOptions git.LoadOptions
	fg          *git.Graph

	ev eval.Eval

	authOpt  *auth.Options
	bqClient *bigquery.Client
}

func (r *createModelRun) validateFlags() error {
	if err := r.ev.ValidateFlags(); err != nil {
		return err
	}
	switch {
	case r.modelDir == "":
		return errors.New("-model-dir is required")
	case r.checkout == "":
		return errors.New("-checkout is required")
	default:
		return nil
	}
}

func (r *createModelRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	if len(args) != 0 {
		return r.done(errors.New("unexpected positional arguments"))
	}
	if err := r.validateFlags(); err != nil {
		return r.done(err)
	}

	var err error
	if r.bqClient, err = newBQClient(ctx, auth.NewAuthenticator(ctx, auth.InteractiveLogin, *r.authOpt)); err != nil {
		return r.done(errors.Annotate(err, "failed to create BigQuery client").Err())
	}

	return r.done(r.writeModel(ctx, r.modelDir))
}

// writeModel writes the model files to the directory.
func (r *createModelRun) writeModel(ctx context.Context, dir string) error {
	// Ensure model dir exists.
	if err := os.MkdirAll(dir, 0777); err != nil {
		return errors.Annotate(err, "failed to create model dir at %q", dir).Err()
	}

	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()

	eg.Go(func() error {
		err := r.writeFileGraphModel(ctx, filepath.Join(dir, "git-file-graph"))
		return errors.Annotate(err, "failed to write file graph model").Err()
	})

	eg.Go(func() error {
		err := r.writeTestFileSet(ctx, filepath.Join(dir, "test-file-set.txt"))
		return errors.Annotate(err, "failed to write test file set").Err()
	})

	return eg.Wait()
}

// writeFileGraphModel writes the file graph model to the model dir.
func (r *createModelRun) writeFileGraphModel(ctx context.Context, dir string) error {
	var err error
	if r.fg, err = git.Load(ctx, r.checkout, r.loadOptions); err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0777); err != nil {
		return err
	}

	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()

	eg.Go(func() error {
		err := r.writeFileGraph(ctx, filepath.Join(dir, "graph.fg"))
		return errors.Annotate(err, "failed to write file graph").Err()
	})

	eg.Go(func() error {
		err := r.writeEvalResults(ctx, filepath.Join(dir, "eval-results.json"))
		return errors.Annotate(err, "failed to write thresholds").Err()
	})

	return eg.Wait()
}

// writeFileGraph writes the graph file.
func (r *createModelRun) writeFileGraph(ctx context.Context, fileName string) error {
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()
	bufW := bufio.NewWriter(f)
	if err := r.fg.Write(bufW); err != nil {
		return err
	}
	return bufW.Flush()
}

// writeEvalResults evaluates the selection strategy, prints results and writes
// them to the file.
func (r *createModelRun) writeEvalResults(ctx context.Context, fileName string) error {
	r.ev.Strategy = r.fg.EvalStrategy
	res, err := r.ev.Run(ctx)
	if err != nil {
		return err
	}

	res.Print(os.Stdout, 0.9)

	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(res)
}

// writeTestFileSet writes the test file set in Chromium to the file.
func (r *createModelRun) writeTestFileSet(ctx context.Context, fileName string) error {
	// Grab all test file names in the past 1 week.
	// Use ci_test_results table (not CQ) because it is smaller and it does not
	// include test file names that never made it to the repo.
	q := r.bqClient.Query(`
		WITH file_names AS (
			SELECT IFNULL(IFNULL(tr.test_metadata.location.file_name, tr.test_location.file_name), '') FileName
			FROM luci-resultdb.chromium.ci_test_results tr
			WHERE partition_time > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
		)
		SELECT DISTINCT FileName
		FROM file_names
		WHERE FileName != ''
		ORDER BY FileName
	`)
	it, err := q.Read(ctx)
	if err != nil {
		return err
	}

	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Read the next row.
		var row struct {
			FileName string
		}
		switch err := it.Next(&row); {
		case err == iterator.Done:
			return nil
		case err != nil:
			return err
		}

		if _, err := fmt.Fprintln(f, row.FileName); err != nil {
			return err
		}
	}
}
