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
	"net/http"
	"strconv"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/stringmapflag"

	"go.chromium.org/luci/led/job"
	"go.chromium.org/luci/led/ledcmd"
)

func getBuildCmd(opts cmdBaseOptions) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-build <buildbucket_build_id>",
		ShortDesc: "obtain a JobDefinition from a buildbucket build",
		LongDesc: `Obtains the build's definition from buildbucket and produces a JobDefinition.

buildbucket_build_id can be specified with "b" prefix like b8962624445013664976,
which is useful when copying it from ci.chromium.org URL.`,

		CommandRun: func() subcommands.CommandRun {
			ret := &cmdGetBuild{}
			ret.initFlags(opts)
			return ret
		},
	}
}

type cmdGetBuild struct {
	cmdBase

	bbHost               string
	pinBotID             bool
	priorityDiff         int
	realBuild            bool
	experiments          stringmapflag.Value
	processedExperiments map[string]bool

	buildID int64
}

func (c *cmdGetBuild) initFlags(opts cmdBaseOptions) {
	c.Flags.StringVar(&c.bbHost, "B", "cr-buildbucket.appspot.com",
		"The buildbucket hostname to grab the definition from.")

	c.Flags.BoolVar(&c.pinBotID, "pin-bot-id", false,
		"Pin the bot id in the generated job Definition's dimensions.")

	c.Flags.IntVar(&c.priorityDiff, "adjust-priority", 10,
		"Increase or decrease the priority of the generated job. Note: priority works like Unix 'niceness'; Higher values indicate lower priority.")

	c.Flags.BoolVar(&c.realBuild, "real-build", true,
		"Get a synthesized build using the provided build as template, instead the provided build itself.")

	c.Flags.Var(&c.experiments, "experiment",
		"Note: only works in real-build mode.\n"+
			"(repeatable) enable or disable an experiment. This takes a parameter of `experiment_name=true|false` and "+
			"adds/removes the corresponding experiment. Already enabled experiments are left as is unless they "+
			"are explicitly disabled.")

	c.cmdBase.initFlags(opts)
}

func (c *cmdGetBuild) jobInput() bool                  { return false }
func (c *cmdGetBuild) positionalRange() (min, max int) { return 1, 1 }

func (c *cmdGetBuild) validateFlags(ctx context.Context, positionals []string, env subcommands.Env) (err error) {
	if err := pingHost(c.bbHost); err != nil {
		return errors.Annotate(err, "buildbucket host").Err()
	}

	buildIDStr := positionals[0]
	if strings.HasPrefix(buildIDStr, "b") {
		// Milo URL structure prefixes buildbucket builds id with "b".
		buildIDStr = positionals[0][1:]
	}
	if c.buildID, err = strconv.ParseInt(buildIDStr, 10, 64); err != nil {
		return errors.Annotate(err, "bad <buildbucket_build_id>").Err()
	}
	if !c.realBuild {
		return errors.Reason("legacy led mode has been deprecated, please trigger led real build instead").Err()
	}
	if c.processedExperiments, err = processExperiments(c.experiments); err != nil {
		return err
	}
	return nil
}

func (c *cmdGetBuild) execute(ctx context.Context, authClient *http.Client, _ auth.Options, inJob *job.Definition) (out any, err error) {
	return ledcmd.GetBuild(ctx, authClient, ledcmd.GetBuildOpts{
		BuildbucketHost: c.bbHost,
		BuildID:         c.buildID,
		PinBotID:        c.pinBotID,
		PriorityDiff:    c.priorityDiff,
		KitchenSupport:  c.kitchenSupport,
		RealBuild:       c.realBuild,
		Experiments:     c.processedExperiments,
	})
}

func (c *cmdGetBuild) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return c.doContextExecute(a, c, args, env)
}
