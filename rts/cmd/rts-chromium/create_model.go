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
)

func cmdCreateModel(authOpt *auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `create-model -model-dir <path>`,
		ShortDesc: "create a model to be used by select subcommand",
		LongDesc: text.Doc(`
			Create a model to be used by select subcommand
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &createModelRun{authOpt: authOpt}
			r.graph.RegisterFlags(&r.Flags)
			r.Flags.StringVar(&r.modelDir, "model-dir", "", text.Doc(`
				Path to the directory where to write model files.
				The directory will be created if it does not exist, or overwritten
				otherwise.
			`))
			return r
		},
	}
}

type createModelRun struct {
	baseCommandRun
	authOpt  *auth.Options
	graph    graphLoader
	modelDir string

	bqClient *bigquery.Client
}

func (r *createModelRun) validateFlags(args []string) error {
	if err := r.graph.ValidateFlags(); err != nil {
		return err
	}
	switch {
	case r.modelDir == "":
		return errors.New("-model-dir is required")
	case len(args) > 0:
		return errors.New("unexpected positional arguments")
	default:
		return nil
	}
}

func (r *createModelRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	if err := r.validateFlags(args); err != nil {
		return r.done(err)
	}

	var err error
	if r.bqClient, err = newBQClient(ctx, auth.NewAuthenticator(ctx, auth.InteractiveLogin, *r.authOpt)); err != nil {
		return r.done(errors.Annotate(err, "failed to create BigQuery client").Err())
	}

	return r.done(r.writeModel(ctx))
}

func (r *createModelRun) writeModel(ctx context.Context) error {
	// Ensure model dir exists.
	if err := os.MkdirAll(r.modelDir, 0777); err != nil {
		return errors.Annotate(err, "failed to create model dir at %q", r.modelDir).Err()
	}

	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()

	eg.Go(func() error {
		err := r.writeTestFileSet(ctx)
		return errors.Annotate(err, "failed to write test file set").Err()
	})

	eg.Go(func() error {
		err := r.writeFileGraphModel(ctx)
		return errors.Annotate(err, "failed to write file graph model").Err()
	})

	return eg.Wait()
}

func (r *createModelRun) writeFileGraphModel(ctx context.Context) error {
	if err := r.graph.Load(ctx); err != nil {
		return err
	}

	dir := filepath.Join(r.modelDir, "git-file-graph")
	if err := os.MkdirAll(dir, 0777); err != nil {
		return err
	}

	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()

	eg.Go(func() error {
		fileName := filepath.Join(dir, "graph.fg")
		err := r.writeFileGraph(ctx, fileName)
		return errors.Annotate(err, "failed to write file graph").Err()
	})

	eg.Go(func() error {
		fileName := filepath.Join(dir, "thresholds.json")
		err := r.writeThresholds(ctx, fileName)
		return errors.Annotate(err, "failed to write thresholds").Err()
	})

	return eg.Wait()
}

func (r *createModelRun) writeFileGraph(ctx context.Context, fileName string) error {
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()
	bufW := bufio.NewWriter(f)
	if err := r.graph.Write(bufW); err != nil {
		return err
	}
	return bufW.Flush()
}

func (r *createModelRun) writeThresholds(ctx context.Context, fileName string) error {
	// TODO(nodir): implement this.
	return nil
}

func (r *createModelRun) writeTestFileSet(ctx context.Context) error {
	fileName := filepath.Join(r.modelDir, "test-file-set.txt")
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()

	q := r.bqClient.Query(`
		WITH file_names AS (
			SELECT IFNULL(IFNULL(tr.test_metadata.location.file_name, tr.test_location.file_name), '') FileName
			FROM luci-resultdb.chromium.ci_test_results tr
			WHERE partition_time > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 14 DAY)
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
