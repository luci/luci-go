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
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/context"

	"github.com/andygrunwald/go-gerrit"
	"github.com/maruel/subcommands"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/led/job"
)

func editCrCLCmd(opts cmdBaseOptions) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "edit-cr-cl [-remove] URL_TO_CHANGELIST",
		ShortDesc: "sets Chromium CL-related properties on this JobDefinition (for experimenting with tryjob recipes)",
		LongDesc: `This allows you to edit a JobDefinition for some tryjob recipe
(e.g. chromium_tryjob), and associate a changelist with it, as if the recipe
was triggered via Gerrit.

Recognized URLs:
	https://<gerrit_host>/c/<path/to/project>/+/<issue>/<patchset>
	https://<gerrit_host>/c/<path/to/project>/+/<issue>/<patchset>

For tasks consuming multiple input CLs, you can adjust which of the CLs you wish
to change by using the "-at-index" flag. By default this command modifies the
first CL on the task.
`,

		CommandRun: func() subcommands.CommandRun {
			ret := &cmdEditCl{}
			ret.initFlags(opts)
			return ret
		},
	}
}

type cmdEditCl struct {
	cmdBase

	gerritChange *bbpb.GerritChange
	remove       bool
}

func (c *cmdEditCl) initFlags(opts cmdBaseOptions) {
	c.Flags.BoolVar(&c.remove, "remove", false, "If provided, will remove the given CL instead of adding it.")
	c.cmdBase.initFlags(opts)
}

func (c *cmdEditCl) jobInput() bool                  { return true }
func (c *cmdEditCl) positionalRange() (min, max int) { return 1, 1 }

func parseCrChangeListURL(clURL string) (*bbpb.GerritChange, error) {
	p, err := url.Parse(clURL)
	if err != nil {
		return nil, errors.Annotate(err, "URL_TO_CHANGELIST is invalid").Err()
	}
	if !strings.HasSuffix(p.Hostname(), "-review.googlesource.com") {
		return nil, errors.Annotate(err, "Only *-review.googlesource.com URLs are supported.").Err()
	}

	var toks []string
	if trimPath := strings.Trim(p.Path, "/"); len(trimPath) > 0 {
		toks = strings.Split(trimPath, "/")
	}

	if len(toks) == 0 {
		// https://<gerrit_host>/#/c/<issue>
		// https://<gerrit_host>/#/c/<issue>/<patchset>
		return nil, errors.Reason("old gerrit URL: %q", clURL).Err()
	} else if toks[0] != "c" {
		return nil, errors.Reason("Unknown changelist URL format: %q", clURL).Err()
	}
	toks = toks[1:] // remove "c"

	// toks ==                 v --------------------------------v
	// https://<gerrit_host>/c/<issue>
	// https://<gerrit_host>/c/<issue>/<patchset>
	// https://<gerrit_host>/c/<project/path>/+/<issue>
	// https://<gerrit_host>/c/<project/path>/+/<issue>/<patchset>

	var projectToks []string
	var issuePatchsetToks []string
	for i, tok := range toks {
		if tok == "+" {
			projectToks, issuePatchsetToks = toks[:i], toks[i+1:]
			break
		}
	}

	if len(projectToks) == 0 {
		return nil, errors.Reason("gerrit URL missing project: %q", clURL).Err()
	}
	if len(issuePatchsetToks) == 0 {
		return nil, errors.Reason("gerrit URL missing issue/patchset: %q", clURL).Err()
	}

	ret := &bbpb.GerritChange{
		Host:    p.Hostname(),
		Project: strings.Join(projectToks, "/"),
	}
	ret.Change, err = strconv.ParseInt(issuePatchsetToks[0], 10, 64)
	if err != nil {
		return nil, errors.Reason("gerrit URL parsing issue %q from %q", issuePatchsetToks[0], clURL).Err()
	}
	if len(issuePatchsetToks) > 1 {
		ret.Patchset, err = strconv.ParseInt(issuePatchsetToks[1], 10, 64)
		if err != nil {
			return nil, errors.Reason("gerrit URL parsing patchset %q from %q", issuePatchsetToks[1], clURL).Err()
		}
	} else {
		gc, err := gerrit.NewClient("https://"+ret.Host, nil)
		if err != nil {
			return nil, errors.Annotate(err, "creating new gerrit client").Err()
		}

		ci, rsp, err := gc.Changes.GetChangeDetail(strconv.FormatInt(ret.Change, 10), &gerrit.ChangeOptions{
			AdditionalFields: []string{"CURRENT_REVISION"}})
		if rsp != nil && rsp.StatusCode == http.StatusUnauthorized {
			return nil, errors.Annotate(err,
				"Gerrit host %q requires authentication and no patchset was provided in CL URL %q. "+
					"Please include the patchset you want in your URL (or `0` to ignore this).",
				ret.Host, clURL,
			).Err()
		}
		if err != nil {
			return nil, errors.Annotate(err, "GetChangeDetail").Err()
		}

		// There's only one.
		for _, rd := range ci.Revisions {
			ret.Patchset = int64(rd.Number)
			break
		}
	}
	return ret, nil
}

func (c *cmdEditCl) validateFlags(ctx context.Context, positionals []string, _ subcommands.Env) (err error) {
	c.gerritChange, err = parseCrChangeListURL(positionals[0])
	return errors.Annotate(err, "invalid URL_TO_CHANGESET").Err()
}

func (c *cmdEditCl) execute(ctx context.Context, _ *http.Client, inJob *job.Definition) (out interface{}, err error) {
	return inJob, inJob.HighLevelEdit(func(je job.HighLevelEditor) {
		if c.remove {
			je.RemoveGerritChange(c.gerritChange)
		} else {
			je.AddGerritChange(c.gerritChange)
		}

		// wipe out all the old properties
		je.Properties(map[string]string{
			"blamelist":            "",
			"buildbucket":          "",
			"issue":                "",
			"patch_gerrit_url":     "",
			"patch_issue":          "",
			"patch_project":        "",
			"patch_ref":            "",
			"patch_repository_url": "",
			"patch_set":            "",
			"patch_storage":        "",
			"patchset":             "",
			"repository":           "",
			"rietveld":             "",
		}, true)
	})
}

func (c *cmdEditCl) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return c.doContextExecute(a, c, args, env)
}
