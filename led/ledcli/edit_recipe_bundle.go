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

package ledcli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/stringmapflag"

	"go.chromium.org/luci/led/job"
	"go.chromium.org/luci/led/ledcmd"
)

func editRecipeBundleCmd(opts cmdBaseOptions) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "edit-recipe-bundle [-O project_id=/path/to/local/repo]*",
		ShortDesc: "isolates recipes and adds them to a JobDefinition",
		LongDesc: `Takes recipes from the current repo (based on cwd), along with
any supplied overrides, and pushes them to the isolate service. The CAS digest
for the recipes will be added to the JobDefinition. If the -property-only flag
is passed or the builder has the "led_builder_is_bootstrapped" property set to
true, the "led_cas_recipe_bundle" property will be set with the CAS digest so
that the build's bootstrapper executable can launch the bundled recipes.

Isolating recipes takes a bit of time, so you may want to save the result
of this command (stdout) to an intermediate file for quick edits.
`,

		CommandRun: func() subcommands.CommandRun {
			ret := &cmdEditRecipeBundle{}
			ret.initFlags(opts)
			return ret
		},
	}
}

type cmdEditRecipeBundle struct {
	cmdBase

	debugSleep time.Duration

	propertyOnly bool

	overrides stringmapflag.Value
}

func (c *cmdEditRecipeBundle) initFlags(opts cmdBaseOptions) {
	c.Flags.Var(&c.overrides, "O",
		"(repeatable) override a repo dependency. Takes a parameter of `project_id=/path/to/local/repo`.")

	c.Flags.DurationVar(&c.debugSleep, "debug-sleep", 0,
		"Injects an extra 'sleep' time into the recipe shim which will sleep for the "+
			"designated amount of time after the recipe completes to allow SSH "+
			"debugging of failed recipe state. This accepts a duration like `2h`. "+
			"Valid units are 's', 'm', or 'h'.")

	c.Flags.BoolVar(&c.propertyOnly, "property-only", false,
		fmt.Sprintf("Pass the CAS blob information as JSON via the %q property and "+
			"preserve the executable of the input job rather than overwriting it. This "+
			"is useful for when `exe` is actually a bootstrap program that you don't "+
			"want to change. The same behavior can be enabled for a build without this "+
			"flag by setting the \"led_builder_is_bootstrapped\" property to true.",
			ledcmd.CASRecipeBundleProperty))

	c.cmdBase.initFlags(opts)
}

func (c *cmdEditRecipeBundle) jobInput() bool                  { return true }
func (c *cmdEditRecipeBundle) positionalRange() (min, max int) { return 0, 0 }

func (c *cmdEditRecipeBundle) validateFlags(ctx context.Context, _ []string, _ subcommands.Env) (err error) {
	for k, v := range c.overrides {
		if k == "" {
			return errors.New("override has empty project_id")
		}
		if v == "" {
			return errors.Fmt("override %q has empty repo path", k)
		}
		v, err = filepath.Abs(v)
		if err != nil {
			return errors.Fmt("override %q: %w", k, err)
		}
		c.overrides[k] = v

		var fi os.FileInfo
		switch fi, err = os.Stat(v); {
		case err != nil:
			return errors.Fmt("override %q: %w", k, err)
		case !fi.IsDir():
			return errors.Fmt("override %q: not a directory", k)
		}
	}

	switch {
	case c.debugSleep == 0:
		// OK

	case c.debugSleep < 0:
		return errors.Fmt("-debug-sleep %q: duration may not be negative", c.debugSleep)

	case c.debugSleep < 10*time.Minute:
		return errors.Fmt("-debug-sleep %q: duration is less than 10 minutes... are you sure you want that?",
			c.debugSleep)
	}

	return
}

func (c *cmdEditRecipeBundle) execute(ctx context.Context, _ *http.Client, authOpts auth.Options, inJob *job.Definition) (out any, err error) {
	return inJob, ledcmd.EditRecipeBundle(ctx, authOpts, inJob, &ledcmd.EditRecipeBundleOpts{
		Overrides:    c.overrides,
		DebugSleep:   c.debugSleep,
		PropertyOnly: c.propertyOnly,
	})
}

func (c *cmdEditRecipeBundle) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return c.doContextExecute(a, c, args, env)
}
