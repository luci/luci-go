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
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/terminal"
	"go.chromium.org/luci/led/job"
	"go.chromium.org/luci/led/ledcmd"
	"go.chromium.org/luci/swarming/client/swarming"
)

func launchCmd(opts cmdBaseOptions) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "launch",
		ShortDesc: "launches a JobDefinition on swarming",
		LongDesc: `Launches a given JobDefinition on swarming.

Example:

led get-builder ... |
  led edit ... |
  led launch

If stdout is not a tty (e.g. a file), this command writes a JSON object
containing information about the launched task to stdout.
`,

		CommandRun: func() subcommands.CommandRun {
			ret := &cmdLaunch{}
			ret.initFlags(opts)
			return ret
		},
	}
}

type cmdLaunch struct {
	cmdBase

	modernize     bool
	dump          bool
	local         bool
	leakDir       string
	noLEDTag      bool
	resultdb      job.RDBEnablement
	realBuild     bool
	boundToParent bool
}

func (c *cmdLaunch) initFlags(opts cmdBaseOptions) {
	c.Flags.BoolVar(&c.modernize, "modernize", false, "Update the launched task to modern LUCI standards.")
	c.Flags.BoolVar(&c.noLEDTag, "no-led-tag", false, "Don't add user_agent:led tag")
	c.Flags.BoolVar(&c.dump, "dump", false, "Dump swarming task to stdout instead of running it.")
	c.Flags.BoolVar(&c.local, "local", false,
		"Execute a buildbucket build locally. This will block until the build completes.")
	c.Flags.StringVar(&c.leakDir, "leak-dir", "", "Base directory to leak when running task locally.")
	c.resultdb = ""
	c.Flags.Var(&c.resultdb, "resultdb", text.Doc(`
		Flag for Swarming/ResultDB integration on the launched task. Can be "on" or "off".
		 If "on", resultdb will be forcefully enabled.
		 If "off", resultdb will be forcefully disabled.
		 If unspecified, resultdb will be enabled if the original build had resultdb enabled.`))
	c.Flags.BoolVar(&c.realBuild, "real-build", false, text.Doc(`
		DEPRECATED: Launch a real Buildbucket build instead of a raw swarming task.
		 If the job definition is for a real build, led will launch a real build regardless of this flag.
		 If the job definition is for a raw swarming task but this flag is set, led launch will fail.`))
	c.Flags.BoolVar(&c.boundToParent, "bound-to-parent", false, text.Doc(`
		If the launched job is bound to its parent or not.
		If true, the launched job CANNOT outlive its parent.
		This flag only has effect if the launched job is a real Buildbucket build.`))
	c.cmdBase.initFlags(opts)
}

func (c *cmdLaunch) jobInput() bool                  { return true }
func (c *cmdLaunch) positionalRange() (min, max int) { return 0, 0 }

func (c *cmdLaunch) validateFlags(ctx context.Context, _ []string, _ subcommands.Env) (err error) {
	return
}

func (c *cmdLaunch) execute(ctx context.Context, authClient *http.Client, _ auth.Options, inJob *job.Definition) (out any, err error) {
	uid, err := ledcmd.GetUID(ctx, c.authenticator)
	if err != nil {
		return nil, err
	}

	switch {
	case c.realBuild == inJob.GetBuildbucket().GetRealBuild():
	case !c.realBuild && inJob.GetBuildbucket().GetRealBuild():
		// Likely for `led get-* -real-build | led launch`.
		// We should allow it and treat it as
		// `led get-* -real-build | led launch -real-build`
		logging.Infof(ctx, "Launching the led job as a real build")
		c.realBuild = true
	case c.realBuild && !inJob.GetBuildbucket().GetRealBuild():
		// Likely for `led get-* | led launch -real-build`.
		// Fail it.
		return nil, errors.New("cannot launch a led real build from a legacy job definition")
	}

	opts := ledcmd.LaunchSwarmingOpts{
		DryRun:           c.dump,
		Local:            c.local,
		LeakDir:          c.leakDir,
		UserID:           uid,
		FinalBuildProto:  "build.proto.json",
		KitchenSupport:   c.kitchenSupport,
		ParentTaskId:     os.Getenv(swarming.TaskIDEnvVar),
		ResultDB:         c.resultdb,
		NoLEDTag:         c.noLEDTag,
		CanOutliveParent: !c.boundToParent,
	}

	buildbucketHostname := inJob.GetBuildbucket().GetBbagentArgs().GetBuild().GetInfra().GetBuildbucket().GetHostname()
	swarmingHostname := inJob.Info().SwarmingHostname()
	miloHost := "ci.chromium.org"
	if strings.Contains(buildbucketHostname, "-dev") || strings.Contains(swarmingHostname, "-dev") {
		miloHost = "luci-milo-dev.appspot.com"
	}

	// Currently modernize only means 'upgrade to bbagent from kitchen'.
	if bb := inJob.GetBuildbucket(); bb != nil {
		if c.modernize {
			bb.LegacyKitchen = false
		}
		if c.realBuild {
			build, err := ledcmd.GetBuildFromJob(inJob, opts)
			if err != nil {
				return nil, err
			}

			switch {
			case opts.DryRun:
				return build, nil
			case opts.Local:
				return ledcmd.LaunchLocalBuild(ctx, build, opts)
			default:
				build, err = ledcmd.LaunchRemoteBuild(ctx, authClient, build, opts)
				if err != nil {
					return nil, err
				}

				logging.Infof(ctx, "LUCI UI: https://%s/b/%d", miloHost, build.Id)
				if !terminal.IsTerminal(int(os.Stdout.Fd())) {
					ret := &struct {
						Buildbucket struct {
							// The id of the launched build.
							BuildID int64 `json:"build_id"`

							// The hostname of the buildbucket server
							Hostname string `json:"host_name"`
						} `json:"buildbucket"`
					}{}

					ret.Buildbucket.BuildID = build.Id
					ret.Buildbucket.Hostname = buildbucketHostname
					return ret, nil
				}
			}
			return nil, nil
		}
	}

	task, meta, err := ledcmd.LaunchSwarming(ctx, authClient, inJob, opts)
	if err != nil {
		return nil, err
	}
	if c.dump {
		return task, nil
	}

	logging.Infof(ctx, "Launched swarming task: https://%s/task?id=%s",
		swarmingHostname, meta.TaskId)
	logging.Infof(ctx, "LUCI UI: https://%s/swarming/task/%s?server=%s",
		miloHost, meta.TaskId, swarmingHostname)

	ret := &struct {
		Swarming struct {
			// The swarming task ID of the launched task.
			TaskID string `json:"task_id"`

			// The hostname of the swarming server
			Hostname string `json:"host_name"`
		} `json:"swarming"`
	}{}

	if !terminal.IsTerminal(int(os.Stdout.Fd())) {
		ret.Swarming.TaskID = meta.TaskId
		ret.Swarming.Hostname = swarmingHostname
	} else {
		ret = nil
	}

	return ret, nil
}

func (c *cmdLaunch) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return c.doContextExecute(a, c, args, env)
}
