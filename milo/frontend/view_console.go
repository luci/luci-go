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
	"encoding/hex"
	"fmt"
	"html/template"
	"net/http"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/templates"

	"github.com/luci/luci-go/milo/api/resp"
	"github.com/luci/luci-go/milo/buildsource"
	"github.com/luci/luci-go/milo/common"
	"github.com/luci/luci-go/milo/common/model"
	"github.com/luci/luci-go/milo/git"
)

// getConsoleDef finds the console definition as defined by any project.
// If the user is not a reader of the project, this will return a 404.
// TODO(hinoka): If the user is not a reader of any of of the builders returned,
// that builder will be removed from list of results.
func getConsoleDef(c context.Context, project, name string) (*common.Console, error) {
	cs, err := common.GetConsole(c, project, name)
	if err != nil {
		return nil, err
	}
	// TODO(hinoka): Remove builders that the user does not have access to.
	return cs, nil
}

func console(c context.Context, project, name string) (*resp.Console, error) {
	tStart := clock.Now(c)
	def, err := getConsoleDef(c, project, name)
	if err != nil {
		return nil, err
	}
	commitInfo, err := git.GetHistory(c, def.RepoURL, def.Ref, 25)
	if err != nil {
		return nil, err
	}
	tGitiles := clock.Now(c)
	logging.Debugf(c, "Loading commits took %s.", tGitiles.Sub(tStart))

	builderNames := make([]string, len(def.Builders))
	builders := make([]resp.BuilderRef, len(def.Builders))
	for i, b := range def.Builders {
		builderNames[i] = b
		builders[i].Name = b
		_, _, builders[i].ShortName, _ = buildsource.BuilderID(b).Split()
		// TODO(hinoka): Add Categories back in.
	}

	commitNames := make([]string, len(commitInfo.Commits))
	for i, commit := range commitInfo.Commits {
		commitNames[i] = hex.EncodeToString(commit.Hash)
	}
	rows, err := buildsource.GetConsoleRows(c, project, def, commitNames, builderNames)
	tConsole := clock.Now(c)
	logging.Debugf(c, "Loading the console took a total of %s.", tConsole.Sub(tGitiles))
	if err != nil {
		return nil, err
	}

	ccb := make([]resp.CommitBuild, len(commitInfo.Commits))
	for row, commit := range commitInfo.Commits {
		ccb[row].Build = make([]*model.BuildSummary, len(builders))
		ccb[row].Commit = resp.Commit{
			AuthorName:  commit.AuthorName,
			AuthorEmail: commit.AuthorEmail,
			CommitTime:  google.TimeFromProto(commit.CommitTime),
			Repo:        def.RepoURL,
			Branch:      def.Ref, // TODO(hinoka): Actually this doesn't match, change branch to ref.
			Description: commit.Msg,
			Revision:    resp.NewLink(commitNames[row], def.RepoURL+"/+/"+commitNames[row]),
		}

		for col, b := range builders {
			name := buildsource.BuilderID(b.Name)
			if summaries := rows[row].Builds[name]; len(summaries) > 0 {
				ccb[row].Build[col] = summaries[0]
			}
		}
	}

	return &resp.Console{
		Name:       def.ID,
		Commit:     ccb,
		BuilderRef: builders,
	}, nil
}

// consoleRenderer is a wrapper around Console to provide additional methods.
type consoleRenderer struct {
	*resp.Console
}

// Header generates the console header html.
func (c consoleRenderer) Header() template.HTML {
	// First, split things into nice rows and find the max depth.
	cat := make([][]string, len(c.BuilderRef))
	depth := 0
	for i, b := range c.BuilderRef {
		cat[i] = b.Category
		if len(cat[i]) > depth {
			depth = len(cat[i])
		}
	}

	result := ""
	for row := 0; row < depth; row++ {
		result += "<tr><th></th><th></th>"
		// "" is the first two nodes, " " is an empty node.
		current := ""
		colspan := 0
		for _, br := range cat {
			colspan++
			var s string
			if row >= len(br) {
				s = " "
			} else {
				s = br[row]
			}
			if s != current || current == " " {
				if current != "" || current == " " {
					result += fmt.Sprintf(`<th colspan="%d">%s</th>`, colspan, current)
					colspan = 0
				}
				current = s
			}
		}
		if colspan != 0 {
			result += fmt.Sprintf(`<th colspan="%d">%s</th>`, colspan, current)
		}
		result += "</tr>"
	}

	// Last row: The actual builder shortnames.
	result += "<tr><th></th><th></th>"
	for _, br := range c.BuilderRef {
		result += fmt.Sprintf("<th>%s</th>", br.ShortName)
	}
	result += "</tr>"
	return template.HTML(result)
}

func (c consoleRenderer) BuilderLink(bs *model.BuildSummary) (*resp.Link, error) {
	_, _, builderName, err := buildsource.BuilderID(bs.BuilderID).Split()
	if err != nil {
		return nil, err
	}
	return resp.NewLink(builderName, "/"+bs.BuilderID), nil
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
		"Console": consoleRenderer{result},
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
