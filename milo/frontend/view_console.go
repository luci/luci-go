// Copyright 2017 The LUCI Authors.
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

package frontend

import (
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/api/gitiles"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/templates"

	"github.com/luci/luci-go/milo/api/config"
	"github.com/luci/luci-go/milo/api/resp"
	"github.com/luci/luci-go/milo/common"
	"github.com/luci/luci-go/milo/common/model"
)

// Returns results of build[commit_index][builder_index]
func getConsoleBuilds(
	c context.Context, builders []resp.BuilderRef, commits []string) (
	[][]*model.BuildSummary, error) {

	panic("Nothing to see here, check back later.")
}

// getConsoleDef finds the console definition as defined by any project.
// If the user is not a reader of the project, this will return a 404.
// TODO(hinoka): If the user is not a reader of any of of the builders returned,
// that builder will be removed from list of results.
func getConsoleDef(c context.Context, project, name string) (*config.Console, error) {
	cs, err := common.GetConsole(c, project, name)
	if err != nil {
		return nil, err
	}
	// TODO(hinoka): Remove builders that the user does not have access to.
	return cs, nil
}

func summaryToConsole(bs []*model.BuildSummary) []*resp.ConsoleBuild {
	cb := make([]*resp.ConsoleBuild, 0, len(bs))
	for _, b := range bs {
		cb = append(cb, &resp.ConsoleBuild{
			// TODO(hinoka): This should link to the actual build.
			Link:   resp.NewLink(b.BuildKey.String(), "#"),
			Status: b.Summary.Status,
		})
	}
	return cb
}

func console(c context.Context, project, name string) (*resp.Console, error) {
	tStart := clock.Now(c)
	def, err := getConsoleDef(c, project, name)
	if err != nil {
		return nil, err
	}
	commits, err := getCommits(c, def.RepoURL, def.Branch, 25)
	if err != nil {
		return nil, err
	}
	tGitiles := clock.Now(c)
	logging.Debugf(c, "Loading commits took %s.", tGitiles.Sub(tStart))
	commitNames := make([]string, len(commits))
	commitLinks := make([]*resp.Link, len(commits))
	for i, commit := range commits {
		commitNames[i] = commit.Revision.Label
		commitLinks[i] = commit.Revision
	}

	// HACK(hinoka): This only supports buildbot....
	builders := make([]resp.BuilderRef, len(def.Builders))
	for i, b := range def.Builders {
		builders[i] = resp.BuilderRef{
			b.Name, strings.Split(b.Category, "|"), b.ShortName,
		}
	}
	cb, err := getConsoleBuilds(c, builders, commitNames)
	tConsole := clock.Now(c)
	logging.Debugf(c, "Loading the console took a total of %s.", tConsole.Sub(tGitiles))
	if err != nil {
		return nil, err
	}
	ccb := make([]resp.CommitBuild, len(commits))
	for i, commit := range commitLinks {
		// TODO(hinoka): Not like this
		ccb[i].Commit = resp.Commit{Revision: commit}
		ccb[i].Build = summaryToConsole(cb[i])
	}

	cs := &resp.Console{
		Name:       def.Name,
		Commit:     ccb,
		BuilderRef: builders,
	}

	return cs, nil
}

func getCommits(c context.Context, repoURL, treeish string, limit int) ([]resp.Commit, error) {
	commits, err := gitiles.Log(c, repoURL, treeish, limit)
	if err != nil {
		return nil, err
	}
	result := make([]resp.Commit, len(commits))
	for i, log := range commits {
		result[i] = resp.Commit{
			AuthorName:  log.Author.Name,
			AuthorEmail: log.Author.Email,
			Repo:        repoURL,
			Revision:    resp.NewLink(log.Commit, repoURL+"/+/"+log.Commit),
			Description: log.Message,
			Title:       strings.SplitN(log.Message, "\n", 2)[0],
			// TODO(hinoka): Fill in the rest of resp.Commit and add those details
			// in the html.
		}
	}
	return result, nil
}

// ConsoleHandler renders the console page.
func ConsoleHandler(c *router.Context) {
	project := c.Params.ByName("project")
	if project == "" {
		ErrorHandler(c, errors.New("Missing Project", common.CodeParameterError))
		return
	}
	name := c.Params.ByName("name")

	result, err := console(c.Context, project, name)
	if err != nil {
		ErrorHandler(c, err)
		return
	}

	templates.MustRender(c.Context, c.Writer, "pages/console.html", templates.Args{
		"Console": result,
	})
}

// ConsoleMainHandler is a redirect handler that redirects the user to the main
// console for a particular project.
func ConsoleMainHandler(ctx *router.Context) {
	w, r, p := ctx.Writer, ctx.Request, ctx.Params
	proj := p.ByName("project")
	http.Redirect(w, r, fmt.Sprintf("/console/%s/main", proj), http.StatusMovedPermanently)
	return
}
