// Copyright 2023 The LUCI Authors.
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
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/led/job"
)

func editGitilesCommitCmd(opts cmdBaseOptions) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "edit-gitiles-commit URL_TO_GITILES_COMMIT",
		ShortDesc: "sets Gitiles Commit properties on this JobDefinition (for experimenting with ci jobs)",
		LongDesc: `This allows you to edit a JobDefinition and associate a Gitiles commit with
it, as if the job was triggered against the commit via CI.

Recognized URL form:
	https://<gitiles_host>/<project>/+/<commit> - require to provide commit ref through -ref flag
	https://<gitiles_host>/<project>/+/<ref>

The provided ref in both cases must start with "refs/".
`,

		CommandRun: func() subcommands.CommandRun {
			ret := &cmdGitilesCommit{}
			ret.initFlags(opts)
			return ret
		},
	}
}

type cmdGitilesCommit struct {
	cmdBase

	gitilesCommit *bbpb.GitilesCommit
	ref           string
}

func (c *cmdGitilesCommit) initFlags(opts cmdBaseOptions) {
	c.Flags.StringVar(&c.ref, "ref", "", text.Doc(`
		Ref of the gitiles commit. Should only be used if the gitiles commit url
		is in the format of https://<gitiles_host>/<project>/+/<commit>.

		Must start with "refs/"
	`))

	c.cmdBase.initFlags(opts)
}

func (c *cmdGitilesCommit) jobInput() bool                  { return true }
func (c *cmdGitilesCommit) positionalRange() (min, max int) { return 1, 1 }

func annotateURLErr(err error) error {
	return errors.WrapIf(err, "invalid URL_TO_GITILES_COMMIT")
}

func annotateRefCmdErr(err error) error {
	return errors.WrapIf(err, "invalid `-ref` flag")
}

func parseGitilesURL(gitilesURL, refCmd string) (*bbpb.GitilesCommit, error) {
	p, err := url.Parse(gitilesURL)
	if err != nil {
		return nil, annotateURLErr(err)
	}
	p.Host = strings.ReplaceAll(p.Host, ".git.corp.google.com", ".googlesource.com")
	if !strings.HasSuffix(p.Hostname(), ".googlesource.com") {
		return nil, annotateURLErr(errors.New("Only *.googlesource.com URLs are supported"))
	}
	if strings.HasSuffix(p.Hostname(), "-review.googlesource.com") {
		return nil, annotateURLErr(errors.New("Please specify Gitiles URL instead of Gerrit URL"))
	}

	var toks []string
	if trimPath := strings.Trim(p.Path, "/"); len(trimPath) > 0 {
		toks = strings.Split(trimPath, "/")
	}

	var projectToks []string
	var commitToks []string

	for i, tok := range toks {
		if tok == "+" {
			projectToks, commitToks = toks[:i], toks[i+1:]
			break
		}
	}
	if len(projectToks) == 0 || len(commitToks) == 0 {
		return nil, annotateURLErr(errors.Fmt("Unknown Gitiles URL format: %q", gitilesURL))
	}

	c := &bbpb.GitilesCommit{
		Host:    p.Hostname(),
		Project: strings.Join(projectToks, "/"),
	}

	if len(commitToks) == 1 {
		c.Id = commitToks[0]
		switch {
		case refCmd == "":
			return nil, annotateRefCmdErr(errors.New("Please provide commit ref through `-ref` flag"))
		case !strings.HasPrefix(refCmd, "refs/"):
			return nil, annotateRefCmdErr(errors.Fmt("Commit ref should start with `refs/`: %q", refCmd))
		default:
			c.Ref = refCmd
		}

	} else {
		ref := strings.Join(commitToks, "/")
		switch {
		case !strings.HasPrefix(ref, "refs/"):
			return nil, annotateURLErr(errors.Fmt("Commit ref should start with `refs/`: %q", ref))
		case refCmd != "":
			return nil, annotateRefCmdErr(errors.New("Please remove `-ref` flag from the command, ref is already found from gitiles url"))
		default:
			c.Ref = ref
		}
	}

	return c, nil
}

func (c *cmdGitilesCommit) validateFlags(ctx context.Context, positionals []string, _ subcommands.Env) (err error) {
	c.gitilesCommit, err = parseGitilesURL(positionals[0], c.ref)
	return err
}

func (c *cmdGitilesCommit) execute(ctx context.Context, _ *http.Client, _ auth.Options, inJob *job.Definition) (out any, err error) {
	return inJob, inJob.HighLevelEdit(func(je job.HighLevelEditor) {
		je.GitilesCommit(c.gitilesCommit)
	})
}

func (c *cmdGitilesCommit) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return c.doContextExecute(a, c, args, env)
}
