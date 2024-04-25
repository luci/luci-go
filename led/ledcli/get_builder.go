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
	"regexp"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/common/flag/stringmapflag"
	"go.chromium.org/luci/led/job"
	"go.chromium.org/luci/led/ledcmd"
)

func getBuilderCmd(opts cmdBaseOptions) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-builder bucket_name:builder_name or project/bucket/builder",
		ShortDesc: "obtain a JobDefinition from a buildbucket builder",
		LongDesc:  `Obtains the builder definition from buildbucket and produces a JobDefinition.`,

		CommandRun: func() subcommands.CommandRun {
			ret := &cmdGetBuilder{}
			ret.initFlags(opts)
			return ret
		},
	}
}

type cmdGetBuilder struct {
	cmdBase

	tags                 stringlistflag.Flag
	bbHost               string
	canary               bool
	priorityDiff         int
	realBuild            bool
	experiments          stringmapflag.Value
	processedExperiments map[string]bool

	project  string
	bucket   string
	builder  string
	bucketV1 string
}

func (c *cmdGetBuilder) initFlags(opts cmdBaseOptions) {
	c.Flags.Var(&c.tags, "t",
		"(repeatable) set tags for this build. Buildbucket expects these to be `key:value`.")
	c.Flags.StringVar(&c.bbHost, "B", "cr-buildbucket.appspot.com",
		"The buildbucket hostname to grab the definition from.")
	c.Flags.BoolVar(&c.canary, "canary", false,
		"Get a 'canary' build, rather than a 'prod' build.")
	c.Flags.IntVar(&c.priorityDiff, "adjust-priority", 10,
		"Increase or decrease the priority of the generated job. Note: priority works like Unix 'niceness'; Higher values indicate lower priority.")
	c.Flags.BoolVar(&c.realBuild, "real-build", false,
		"Get a synthesized build for the builder, instead of the swarmbucket template.")
	c.Flags.Var(&c.experiments, "experiment",
		"Note: only works in real-build mode.\n"+
			"(repeatable) enable or disable an experiment. This takes a parameter of `experiment_name=true|false` and "+
			"adds/removes the corresponding experiment. Already enabled experiments are left as is unless they "+
			"are explicitly disabled.")
	c.cmdBase.initFlags(opts)
}

func (c *cmdGetBuilder) jobInput() bool                  { return false }
func (c *cmdGetBuilder) positionalRange() (min, max int) { return 1, 1 }

type builder struct {
	project  string
	v1Bucket string
	v2Bucket string
	builder  string
}

// parseV1Builder parses the builder string in the format of "bucket:builder".
// Where the bucket can have two formats:
// * luci.project.bucket
// * project/bucket
func parseV1Builder(builderStr string) *builder {
	toks := strings.SplitN(builderStr, ":", 2)
	if len(toks) != 2 {
		return nil
	}

	bucket, bldr := toks[0], toks[1]
	if bucket == "" || bldr == "" {
		return nil
	}

	parsed := &builder{
		v1Bucket: bucket,
		builder:  bldr,
	}

	// luci.project.bucket:builder
	v1BucketRe := regexp.MustCompile(`^luci\.([a-z0-9\-_]*)\.([a-z0-9\-_]*)$`)
	match := v1BucketRe.FindStringSubmatch(bucket)
	if len(match) == 3 {
		parsed.project = match[1]
		parsed.v2Bucket = match[2]
		return parsed
	}

	// project/bucket:builder
	subs := strings.Split(bucket, "/")
	if len(subs) != 2 {
		return nil
	}
	parsed.project = subs[0]
	parsed.v2Bucket = subs[1]
	return parsed
}

// parseV2Builder parses the builder string in the format of "project/bucket/builder".
func parseV2Builder(builderStr string) *builder {
	builderID, err := protoutil.ParseBuilderID(builderStr)
	if err != nil {
		return nil
	}
	return &builder{
		project:  builderID.Project,
		v1Bucket: fmt.Sprintf("luci.%s.%s", builderID.Project, builderID.Bucket),
		v2Bucket: builderID.Bucket,
		builder:  builderID.Builder,
	}
}

func (c *cmdGetBuilder) validateFlags(ctx context.Context, positionals []string, env subcommands.Env) (err error) {
	if err := pingHost(c.bbHost); err != nil {
		return errors.Annotate(err, "buildbucket host").Err()
	}

	bldr := parseV1Builder(positionals[0])
	if bldr == nil {
		bldr = parseV2Builder(positionals[0])
	}
	if bldr == nil {
		err = errors.Reason("cannot parse builder: %q", positionals[0]).Err()
		return
	}

	c.builder = bldr.builder
	c.project = bldr.project
	c.bucket = bldr.v2Bucket
	c.bucketV1 = bldr.v1Bucket
	if c.bucket == "" && c.bucketV1 == "" {
		return errors.New("empty bucket")
	}
	if c.builder == "" {
		return errors.New("empty builder")
	}
	if !c.realBuild && len(c.experiments) > 0 {
		return errors.Reason("setting experiments only works in real-build mode.").Err()
	}
	if c.processedExperiments, err = processExperiments(c.experiments); err != nil {
		return err
	}
	return nil
}

func (c *cmdGetBuilder) execute(ctx context.Context, authClient *http.Client, _ auth.Options, inJob *job.Definition) (out any, err error) {
	return ledcmd.GetBuilder(ctx, authClient, ledcmd.GetBuildersOpts{
		BuildbucketHost: c.bbHost,
		Project:         c.project,
		Bucket:          c.bucket,
		Builder:         c.builder,
		Canary:          c.canary,
		ExtraTags:       c.tags,
		PriorityDiff:    c.priorityDiff,
		Experiments:     c.processedExperiments,

		KitchenSupport: c.kitchenSupport,
		RealBuild:      c.realBuild,

		BucketV1: c.bucketV1,
	})
}

func (c *cmdGetBuilder) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return c.doContextExecute(a, c, args, env)
}
