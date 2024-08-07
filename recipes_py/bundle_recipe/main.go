// Copyright 2024 The LUCI Authors.
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
	"flag"
	"fmt"
	"os"

	"go.chromium.org/luci/common/flag/stringmapflag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	recipespy "go.chromium.org/luci/recipes_py"
)

var (
	verbose   = flag.Bool("verbose", false, "print debug level logging to stderr")
	repoRoot  = flag.String("repo-root", ".", "path to the root of the repo to bundle. default is cwd")
	dest      = flag.String("dest", "./bundle", "the directory of where to put the bundle. default is ./bundle")
	overrides = stringmapflag.Value{}
)

func main() {
	flag.Var(
		&overrides,
		"overrides",
		"local override of dependency recipe repos.")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, `Bundle a recipe repo
usage: recipe_bundle [flags]

Flags:`)
		flag.PrintDefaults()
	}

	flag.Parse()

	ctx := context.Background()
	goLoggerCfg := gologger.LoggerConfig{Out: os.Stderr}
	goLoggerCfg.Format = "[%{level:.1s} %{time:2006-01-02 15:04:05}] %{message}"
	ctx = goLoggerCfg.Use(ctx)

	if *verbose {
		ctx = (&logging.Config{Level: logging.Debug}).Set(ctx)
	} else {
		ctx = (&logging.Config{Level: logging.Info}).Set(ctx)
	}

	if err := recipespy.Bundle(ctx, *repoRoot, *dest, overrides); err != nil {
		logging.Errorf(ctx, "fail to bundle the recipe repo. Reason: %s", err)
		os.Exit(1)
	}
}
