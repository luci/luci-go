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

package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/fixflagpos"
	"go.chromium.org/luci/common/logging/gologger"

	"go.chromium.org/luci/rts/filegraph"
	"go.chromium.org/luci/rts/filegraph/git"
)

var logCfg = gologger.LoggerConfig{
	Format: `%{message}`,
	Out:    os.Stderr,
}

type baseCommandRun struct {
	subcommands.CommandRunBase
}

func (r *baseCommandRun) done(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	return 0
}

// Main runs the filegraph program.
func Main() {
	app := &cli.Application{
		Name:  "filegraph",
		Title: "Derive and analyze a directed file graph where an edge weight represens relevance.",
		Context: func(ctx context.Context) context.Context {
			return logCfg.Use(ctx)
		},
		Commands: []*subcommands.Command{
			cmdQuery,

			{},
			subcommands.CmdHelp,
		},
	}

	os.Exit(subcommands.Run(app, fixflagpos.FixSubcommands(os.Args[1:])))
}

// filesToPaths converts file paths to graph node paths.
// If the files belong to different git repositories, returns an error.
func filesToPaths(filePaths ...string) (repoDir string, paths []filegraph.Name, err error) {
	if repoDir, err = ensureSameRepo(filePaths...); err != nil {
		return
	}

	prefix := filepath.ToSlash(filepath.Clean(repoDir))

	paths = make([]filegraph.Name, len(filePaths))
	for i, f := range filePaths {
		if f, err = filepath.Abs(f); err != nil {
			return
		}
		f = filepath.Clean(f)
		f = filepath.ToSlash(f)
		rel := strings.TrimPrefix(f, prefix)
		rel = strings.Trim(rel, "/")
		paths[i] = filegraph.ParseName(rel)
	}
	return
}

// ensureSameRepo ensures that all files belong to the same git repository
// and returns an absolute path to the .git directory.
func ensureSameRepo(files ...string) (repoDir string, err error) {
	if len(files) == 0 {
		return "", errors.New("no files")
	}
	for _, f := range files {
		switch rd, err := git.RepoDir(f); {
		case err != nil:
			return "", err
		case repoDir == "":
			repoDir = rd
		case repoDir != rd:
			return "", errors.Reason("%q and %q reside in different git repositories", files[0], f).Err()
		}
	}
	return repoDir, nil
}
