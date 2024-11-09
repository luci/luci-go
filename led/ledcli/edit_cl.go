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
	"net/url"
	"strconv"
	"strings"

	"github.com/maruel/subcommands"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	gerritapi "go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	"go.chromium.org/luci/led/job"
)

func editGerritCLCmd(opts cmdBaseOptions) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "edit-gerrit-cl [-remove|-no-implicit-clear] URL_TO_CHANGELIST",
		ShortDesc: "sets Gerrit CL-related properties on this JobDefinition (for experimenting with tryjobs)",
		LongDesc: `This allows you to edit a JobDefinition and associate a changelist with
it, as if the job was triggered via Gerrit.

Recognized URL forms:
	https://<gerrit_host>/<change>
	https://<gerrit_host>/c/<path/to/project>/+/<change>
	https://<gerrit_host>/c/<path/to/project>/+/<change>/<patchset>

If you provide URLs in one of the first two forms and <gerrit_host> has public read
access, this will fill in the missing information for the change. Otherwise, this will
fail and ask you to provide the full URL containing host, project, change and patchset.

By default, when adding a CL, this will clear all existing CLs on the job, unless
you pass -no-implicit-clear. Most jobs only expect one CL, so this implicit clearing
behavior is for CLI ergonomic reasons.
`,

		CommandRun: func() subcommands.CommandRun {
			ret := &cmdEditCl{}
			ret.initFlags(opts)
			return ret
		},
	}
}

func editCrCLCmd(opts cmdBaseOptions) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "edit-cr-cl [-remove|-no-implicit-clear] URL_TO_CHANGELIST",
		ShortDesc: "DEPRECATED: sets Gerrit CL-related properties on this JobDefinition (for experimenting with tryjobs)",
		LongDesc: `This command functions identically to edit-gerrit-cl but has a Chrome-specific
name for historical reasons. It's kept for backwards compatibility. Prefer to use edit-gerrit-cl instead.
`,
		Advanced: true,
		CommandRun: func() subcommands.CommandRun {
			ret := &cmdEditCl{}
			ret.initFlags(opts)
			ret.printDeprecationWarning = true
			return ret
		},
	}
}

type cmdEditCl struct {
	cmdBase

	gerritChange            *bbpb.GerritChange
	remove                  bool
	noImplicitClear         bool
	printDeprecationWarning bool
}

func (c *cmdEditCl) initFlags(opts cmdBaseOptions) {
	c.Flags.BoolVar(&c.remove, "remove", false, "If provided, will remove the given CL instead of adding it.")
	c.Flags.BoolVar(&c.noImplicitClear, "no-implicit-clear", false,
		"If provided, will not clear existing CLs when adding a new one.")
	c.cmdBase.initFlags(opts)
}

func (c *cmdEditCl) jobInput() bool                  { return true }
func (c *cmdEditCl) positionalRange() (min, max int) { return 1, 1 }

type changeResolver func(host string, change int64) (proj string, ps int64, err error)

func parseCrChangeListURL(clURL string, resolveChange changeResolver) (*bbpb.GerritChange, error) {
	p, err := url.Parse(clURL)
	if err != nil {
		return nil, errors.Annotate(err, "URL_TO_CHANGELIST").Err()
	}
	p.Host = strings.ReplaceAll(p.Host, ".git.corp.google.com", ".googlesource.com")
	if !strings.HasSuffix(p.Hostname(), "-review.googlesource.com") {
		return nil, errors.New("only *-review.googlesource.com URLs are supported")
	}

	var toks []string
	if trimPath := strings.Trim(p.Path, "/"); len(trimPath) > 0 {
		toks = strings.Split(trimPath, "/")
	}

	if len(toks) == 0 {
		// https://<gerrit_host>/#/c/<change>
		// https://<gerrit_host>/#/c/<change>/<patchset>
		return nil, errors.Reason("old/empty gerrit URL: %q", clURL).Err()
	}

	var projectToks []string
	var changePatchsetToks []string

	if toks[0] == "c" {
		toks = toks[1:] // remove "c"
		// toks ==                 v----------------------------------v
		// https://<gerrit_host>/c/<change>
		// https://<gerrit_host>/c/<change>/<patchset>
		// https://<gerrit_host>/c/<project/path>/+/<change>
		// https://<gerrit_host>/c/<project/path>/+/<change>/<patchset>
		for i, tok := range toks {
			if tok == "+" {
				projectToks, changePatchsetToks = toks[:i], toks[i+1:]
				break
			}
		}
		if len(projectToks) == 0 {
			return nil, errors.Reason("gerrit URL missing project: %q", clURL).Err()
		}
	} else if len(toks) == 1 {
		// toks ==               v------v
		// https://<gerrit_host>/<change>
		changePatchsetToks = toks
	} else {
		return nil, errors.Reason("Unknown changelist URL format: %q", clURL).Err()
	}

	if len(changePatchsetToks) == 0 {
		return nil, errors.Reason("gerrit URL missing change/patchset: %q", clURL).Err()
	}

	ret := &bbpb.GerritChange{
		Host:    p.Hostname(),
		Project: strings.Join(projectToks, "/"),
	}
	ret.Change, err = strconv.ParseInt(changePatchsetToks[0], 10, 64)
	if err != nil {
		return nil, errors.Reason("gerrit URL parsing change %q from %q", changePatchsetToks[0], clURL).Err()
	}
	if len(changePatchsetToks) > 1 {
		ret.Patchset, err = strconv.ParseInt(changePatchsetToks[1], 10, 64)
		if err != nil {
			return nil, errors.Reason("gerrit URL parsing patchset %q from %q", changePatchsetToks[1], clURL).Err()
		}
	} else {
		ret.Project, ret.Patchset, err = resolveChange(ret.Host, ret.Change)
		if err != nil {
			return nil, errors.Annotate(
				err, "resolving patchset from Gerrit Url %q", clURL).Err()
		}
	}

	return ret, nil
}

func gerritResolver(ctx context.Context) changeResolver {
	return func(host string, change int64) (string, int64, error) {
		// TODO(crbug/1211623): allow authentication for internal hosts.
		gc, err := gerritapi.NewRESTClient(http.DefaultClient, host, false)
		if err != nil {
			return "", 0, errors.Annotate(err, "creating new gerrit client").Err()
		}
		ci, err := gc.GetChange(ctx, &gerritpb.GetChangeRequest{
			Number: change,
			Options: []gerritpb.QueryOption{
				gerritpb.QueryOption_CURRENT_REVISION,
			},
		})
		if status.Code(err) == codes.Unauthenticated {
			return "", 0, errors.Annotate(err,
				"Gerrit host %q requires authentication and URL did not include project and/or patchset. "+
					"Please include the project and patchset you want in your URL (patchset can be "+
					"ignored by setting it to `0`).", host,
			).Err()
		}
		if err != nil {
			return "", 0, errors.Annotate(err, "GetChange").Err()
		}

		// There's only one.
		for _, rd := range ci.Revisions {
			return ci.Project, int64(rd.Number), nil
		}
		panic("impossible")
	}
}

func (c *cmdEditCl) validateFlags(ctx context.Context, positionals []string, _ subcommands.Env) (err error) {
	if c.remove && c.noImplicitClear {
		return errors.New("cannot specify both -remove and -no-implicit-clear")
	}

	c.gerritChange, err = parseCrChangeListURL(positionals[0], gerritResolver(ctx))
	return errors.Annotate(err, "invalid URL_TO_CHANGESET").Err()
}

func (c *cmdEditCl) execute(ctx context.Context, _ *http.Client, _ auth.Options, inJob *job.Definition) (out any, err error) {
	if c.printDeprecationWarning {
		logging.Warningf(ctx, "'edit-cr-cl' is a deprecated alias, please use 'edit-gerrit-cl'.")
	}
	return inJob, inJob.HighLevelEdit(func(je job.HighLevelEditor) {
		if c.remove {
			je.RemoveGerritChange(c.gerritChange)
		} else {
			if !c.noImplicitClear {
				je.ClearGerritChanges()
			}
			je.AddGerritChange(c.gerritChange)
		}
	})
}

func (c *cmdEditCl) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return c.doContextExecute(a, c, args, env)
}
