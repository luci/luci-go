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
	"net/http"
	"strings"

	"golang.org/x/net/context"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/led/job"
	"go.chromium.org/luci/led/ledcmd"
)

func getBuilderCmd(opts cmdBaseOptions) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-builder bucket_name:builder_name",
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

	tags   stringlistflag.Flag
	bbHost string
	canary bool

	bucket  string
	builder string
}

func (c *cmdGetBuilder) initFlags(opts cmdBaseOptions) {
	c.Flags.Var(&c.tags, "t",
		"(repeatable) set tags for this build. Buildbucket expects these to be `key:value`.")
	c.Flags.StringVar(&c.bbHost, "B", "cr-buildbucket.appspot.com",
		"The buildbucket hostname to grab the definition from.")
	c.Flags.BoolVar(&c.canary, "canary", false,
		"Get a 'canary' build, rather than a 'prod' build.")

	c.cmdBase.initFlags(opts)
}

func (c *cmdGetBuilder) jobInput() bool                  { return false }
func (c *cmdGetBuilder) positionalRange() (min, max int) { return 1, 1 }

func (c *cmdGetBuilder) validateFlags(ctx context.Context, positionals []string, env subcommands.Env) (err error) {
	if err := validateHost(c.bbHost); err != nil {
		return errors.Annotate(err, "buildbucket host").Err()
	}

	toks := strings.SplitN(positionals[0], ":", 2)
	if len(toks) != 2 {
		err = errors.Reason("cannot parse bucket:builder: %q", positionals[0]).Err()
		return
	}

	c.bucket, c.builder = toks[0], toks[1]
	if c.bucket == "" {
		return errors.New("empty bucket")
	}
	if c.builder == "" {
		return errors.New("empty builder")
	}
	return nil
}

func (c *cmdGetBuilder) execute(ctx context.Context, authClient *http.Client, inJob *job.Definition) (out interface{}, err error) {
	return ledcmd.GetBuilder(ctx, authClient, ledcmd.GetBuildersOpts{
		BuildbucketHost: c.bbHost,
		Bucket:          c.bucket,
		Builder:         c.builder,
		Canary:          c.canary,
		ExtraTags:       c.tags,
	})
}

func (c *cmdGetBuilder) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return c.doContextExecute(a, c, args, env)
}
