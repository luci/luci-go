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
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/memcache"
	"go.chromium.org/gae/service/urlfetch"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/milo/api/config"
	"go.chromium.org/luci/milo/buildsource"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/milo/frontend/ui"
	"go.chromium.org/luci/milo/git"
)

func logTimer(c context.Context, message string) func() {
	tStart := clock.Now(c)
	return func() {
		logging.Debugf(c, "%s: took %s", message, clock.Since(c, tStart))
	}
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
	return &cs.Def, nil
}

// validateFaviconURL checks to see if the URL is well-formed and the host is in
// the whitelist.
func validateFaviconURL(faviconURL string) error {
	parsedFaviconURL, err := url.Parse(faviconURL)
	if err != nil {
		return err
	}
	host := strings.SplitN(parsedFaviconURL.Host, ":", 2)[0]
	if host != "storage.googleapis.com" {
		return fmt.Errorf("%q is not a valid FaviconURL hostname", host)
	}
	return nil
}

func getFaviconURL(c context.Context, def *config.Console) string {
	faviconURL := def.FaviconUrl
	if err := validateFaviconURL(faviconURL); err != nil {
		logging.WithError(err).Warningf(c, "invalid favicon URL")
		faviconURL = ""
	}
	return faviconURL
}

// validateConsoleID checks to see whether a console ID has both a project and
// name, and that the project is the same as the current project being checked.
func validateConsoleID(consoleID, project string) (cid common.ConsoleID, err error) {
	cid, err = common.ParseConsoleID(consoleID)
	if err != nil {
		err = errors.Annotate(err, "console ID must have format <project>/<name>: %s", consoleID).Err()
		return
	}
	if cid.Project != project {
		// TODO(hinoka): Support cross-project consoles.
		err = errors.Reason("console ID is from a different project").Err()
	}
	return
}

type builderRefFactory func(name, shortname string) *ui.BuilderRef

func buildTreeFromDef(def *config.Console, factory builderRefFactory) (*ui.Category, int) {
	// Build console table tree from builders.
	categoryTree := ui.NewCategory("")
	depth := 0
	for _, b := range def.Builders {
		builderRef := factory(b.Name[0], b.ShortName)
		categories := b.ParseCategory()
		if len(categories) > depth {
			depth = len(categories)
		}
		categoryTree.AddBuilder(categories, builderRef)
	}
	return categoryTree, depth
}

// extractBuilderSummaries extracts the builder summaries for the current console
// out of the console summary map.
func extractBuilderSummaries(
	id common.ConsoleID, summaries map[common.ConsoleID]ui.ConsoleSummary) map[string]*model.BuilderSummary {

	consoleSummary, ok := summaries[id]
	if !ok {
		return nil
	}
	builderSummaries := make(map[string]*model.BuilderSummary, len(consoleSummary.Builders))
	for _, builder := range consoleSummary.Builders {
		builderSummaries[builder.BuilderID] = builder
	}
	return builderSummaries
}

// getConsoleGroups extracts the console summaries for all header summaries
// out of the summaries map into console groups for the header.
func getConsoleGroups(def *config.Header, summaries map[common.ConsoleID]ui.ConsoleSummary) []ui.ConsoleGroup {
	if def == nil || len(def.GetConsoleGroups()) == 0 {
		// No header, no console groups.
		return nil
	}
	groups := def.GetConsoleGroups()
	consoleGroups := make([]ui.ConsoleGroup, len(groups))
	for i, group := range groups {
		groupSummaries := make([]ui.ConsoleSummary, len(group.ConsoleIds))
		for j, id := range group.ConsoleIds {
			cid, err := common.ParseConsoleID(id)
			if err != nil {
				// This should never happen, the consoleID was already validated further
				// upstream.
				panic(err)
			}
			if consoleSummary, ok := summaries[cid]; ok {
				groupSummaries[j] = consoleSummary
			} else {
				// This should never happen.
				panic(fmt.Sprintf("could not find console summary %s", id))
			}
		}
		consoleGroups[i].Consoles = groupSummaries
		if group.Title != nil {
			ariaLabel := "console group " + group.Title.Text
			consoleGroups[i].Title = ui.NewLink(group.Title.Text, group.Title.Url, ariaLabel)
			consoleGroups[i].Title.Alt = group.Title.Alt
		}
	}
	return consoleGroups
}

func consoleRowCommits(c context.Context, project string, def *config.Console, limit int) (
	[]*buildsource.ConsoleRow, []ui.Commit, error) {
	tGitiles := logTimer(c, "Rows: loading commit from gitiles")
	commitInfo, err := git.GetHistory(c, def.RepoUrl, def.Ref, limit)
	if err != nil {
		return nil, nil, err
	}
	tGitiles()

	commitNames := make([]string, len(commitInfo.Commits))
	for i, commit := range commitInfo.Commits {
		commitNames[i] = hex.EncodeToString(commit.Hash)
	}

	tBuilds := logTimer(c, "Rows: loading builds")
	rows, err := buildsource.GetConsoleRows(c, project, def, commitNames)
	tBuilds()
	if err != nil {
		return nil, nil, err
	}

	// Build list of commits.
	commits := make([]ui.Commit, len(commitInfo.Commits))
	for row, commit := range commitInfo.Commits {
		commits[row] = ui.Commit{
			AuthorName:  commit.AuthorName,
			AuthorEmail: commit.AuthorEmail,
			CommitTime:  google.TimeFromProto(commit.CommitTime),
			Repo:        def.RepoUrl,
			Branch:      def.Ref, // TODO(hinoka): Actually this doesn't match, change branch to ref.
			Description: commit.Msg,
			Revision:    ui.NewLink(commitNames[row], def.RepoUrl+"/+/"+commitNames[row], fmt.Sprintf("commit by %s", commit.AuthorEmail)),
		}
	}

	return rows, commits, nil
}

// summaries fetches all of the console summaries in the header, in addition
// to the ones for the current console.
func summaries(c context.Context, consoleID common.ConsoleID, def *config.Header) (
	map[common.ConsoleID]ui.ConsoleSummary, error) {

	var ids []common.ConsoleID
	var err error
	if def != nil {
		if ids, err = consoleHeaderGroupIDs(consoleID.Project, def.GetConsoleGroups()); err != nil {
			return nil, err
		}
	}
	ids = append(ids, consoleID)
	return buildsource.GetConsoleSummariesFromIDs(c, ids)
}

func console(c context.Context, project, id string, limit int) (*ui.Console, error) {
	def, err := getConsoleDef(c, project, id)
	if err != nil {
		return nil, err
	}

	consoleID := common.ConsoleID{Project: project, ID: id}
	header := &ui.ConsoleHeader{}
	var rows []*buildsource.ConsoleRow
	var commits []ui.Commit
	var consoleSummaries map[common.ConsoleID]ui.ConsoleSummary
	// Get 3 things in parallel:
	// 1. The console header (except for summaries)
	// 2. The console header summaries + this console's builders summaries.
	// 3. The console body (rows + commits)
	if err := parallel.FanOutIn(func(ch chan<- func() error) {
		if def.Header != nil {
			ch <- func() (err error) {
				defer logTimer(c, "header")()
				header, err = consoleHeader(c, project, def.Header)
				return
			}
		}
		ch <- func() (err error) {
			defer logTimer(c, "summaries")()
			consoleSummaries, err = summaries(c, consoleID, def.Header)
			return
		}
		ch <- func() (err error) {
			defer logTimer(c, "rows")()
			rows, commits, err = consoleRowCommits(c, project, def, limit)
			return
		}
	}); err != nil {
		return nil, err
	}

	// Reassemble builder summaries into both the current console
	// and also the header.
	header.ConsoleGroups = getConsoleGroups(def.Header, consoleSummaries)

	// Reassemble the builder summaries and rows into the categoryTree.
	builderSummaries := extractBuilderSummaries(consoleID, consoleSummaries)
	categoryTree, depth := buildTreeFromDef(def, func(name, shortname string) *ui.BuilderRef {
		// Group together all builds for this builder.
		builds := make([]*model.BuildSummary, len(commits))
		id := buildsource.BuilderID(name)
		for row := 0; row < len(commits); row++ {
			if summaries := rows[row].Builds[id]; len(summaries) > 0 {
				builds[row] = summaries[0]
			}
		}
		builder, ok := builderSummaries[name]
		if !ok {
			logging.Warningf(c, "could not find builder summary for %s", name)
		}
		return &ui.BuilderRef{
			Name:      name,
			ShortName: shortname,
			Build:     builds,
			Builder:   builder,
		}
	})

	return &ui.Console{
		Name:       def.Name,
		Project:    project,
		Header:     header,
		Commit:     commits,
		Table:      *categoryTree,
		MaxDepth:   depth + 1,
		FaviconURL: getFaviconURL(c, def),
	}, nil
}

func consolePreview(c context.Context, summaries ui.ConsoleSummary, def *config.Console) (*ui.Console, error) {
	builderSummaries := make(map[string]*model.BuilderSummary, len(summaries.Builders))
	for _, b := range summaries.Builders {
		builderSummaries[b.BuilderID] = b
	}
	categoryTree, depth := buildTreeFromDef(def, func(name, shortname string) *ui.BuilderRef {
		builder, ok := builderSummaries[name]
		if !ok {
			logging.Warningf(c, "could not find builder summary for %s", name)
		}
		return &ui.BuilderRef{
			Name:      name,
			ShortName: shortname,
			Builder:   builder,
		}
	})
	return &ui.Console{
		Name:       def.Name,
		Table:      *categoryTree,
		MaxDepth:   depth + 1,
		FaviconURL: getFaviconURL(c, def),
	}, nil
}

// getJSONData fetches data from the given URL into the target, caching it
// for expiration seconds in memcache.
func getJSONData(c context.Context, url string, expiration time.Duration, target interface{}) error {
	// Try memcache first.
	item := memcache.NewItem(c, "url:"+url)
	if err := memcache.Get(c, item); err == nil {
		if err = json.Unmarshal(item.Value(), target); err == nil {
			return nil
		}
		logging.WithError(err).Warningf(c, "couldn't load stored item from datastore")
	} else if err != memcache.ErrCacheMiss {
		logging.WithError(err).Errorf(c, "memcache is having issues")
	}

	// Fetch it from the app.
	transport := urlfetch.Get(c)
	response, err := (&http.Client{Transport: transport}).Get(url)
	if err != nil {
		return errors.Annotate(err, "failed to get data from %q", url).Err()
	}
	defer response.Body.Close()

	bytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return errors.Annotate(err, "failed to read response body from %q", url).Err()
	}

	// Parse JSON into target.
	if err := json.Unmarshal(bytes, target); err != nil {
		return errors.Annotate(
			err, "failed to decode JSON %q from %q", bytes, url).Err()
	}

	// Save data into memcache on a best-effort basis.
	item.SetValue(bytes).SetExpiration(expiration)
	if err = memcache.Set(c, item); err != nil {
		logging.WithError(err).Warningf(c, "could not save url data %q into memcache", url)
	}
	return nil
}

// getTreeStatus returns the current tree status from the chromium-status app.
// This never errors, instead it constructs a fake purple TreeStatus
func getTreeStatus(c context.Context, host string) *ui.TreeStatus {
	q := url.Values{}
	q.Add("format", "json")
	url := url.URL{
		Scheme:   "https",
		Host:     host,
		Path:     "current",
		RawQuery: q.Encode(),
	}

	status := &ui.TreeStatus{}
	if err := getJSONData(c, url.String(), 30*time.Second, status); err != nil {
		// Generate a fake tree status.
		logging.WithError(err).Errorf(c, "loading tree status")
		status = &ui.TreeStatus{
			GeneralState: "maintenance",
			Message:      "could not load tree status",
		}
	}

	return status
}

// getOncallData fetches oncall data and caches it for 10 minutes.
func getOncallData(c context.Context, name, url string) (ui.Oncall, error) {
	result := ui.Oncall{}
	err := getJSONData(c, url, 10*time.Minute, &result)
	// Set the name, this is not loaded from the sheriff JSON.
	result.Name = name
	return result, err
}

func consoleHeaderOncall(c context.Context, config []*config.Oncall) ([]ui.Oncall, error) {
	// Get oncall data from URLs.
	oncalls := make([]ui.Oncall, len(config))
	err := parallel.FanOutIn(func(ch chan<- func() error) {
		for i, oc := range config {
			i := i
			oc := oc
			ch <- func() (err error) {
				oncalls[i], err = getOncallData(c, oc.Name, oc.Url)
				return
			}
		}
	})
	return oncalls, err
}

// consoleHeaderGroupIDs extracts the console group IDs out of the header config.
func consoleHeaderGroupIDs(project string, config []*config.ConsoleSummaryGroup) ([]common.ConsoleID, error) {
	consoleIDSet := map[common.ConsoleID]bool{}
	for _, group := range config {
		for _, id := range group.ConsoleIds {
			// TODO(hinoka): Implement proper ACL checking, which will allow cross-project
			// console headers.  The following will be swapped out for an ACL check.
			if cid, err := validateConsoleID(id, project); err != nil {
				return nil, err
			} else {
				consoleIDSet[cid] = true
			}
		}
	}
	consoleIDs := make([]common.ConsoleID, 0, len(consoleIDSet))
	for cid := range consoleIDSet {
		consoleIDs = append(consoleIDs, cid)
	}
	return consoleIDs, nil
}

func consoleHeader(c context.Context, project string, header *config.Header) (*ui.ConsoleHeader, error) {
	// Return nil if the header is empty.
	switch {
	case len(header.Oncalls) != 0:
		// continue
	case len(header.Links) != 0:
		// continue
	default:
		return nil, nil
	}

	var oncalls []ui.Oncall
	var treeStatus *ui.TreeStatus
	// Get the oncall and tree status concurrently.
	if err := parallel.FanOutIn(func(ch chan<- func() error) {
		ch <- func() (err error) {
			oncalls, err = consoleHeaderOncall(c, header.Oncalls)
			if err != nil {
				logging.WithError(err).Errorf(c, "getting oncalls")
			}
			return nil
		}
		if header.TreeStatusHost != "" {
			ch <- func() error {
				treeStatus = getTreeStatus(c, header.TreeStatusHost)
				treeStatus.URL = &url.URL{Scheme: "https", Host: header.TreeStatusHost}
				return nil
			}
		}
	}); err != nil {
		return nil, err
	}

	// Restructure links as resp data structures.
	//
	// This should be a one-to-one transformation.
	links := make([]ui.LinkGroup, len(header.Links))
	for i, linkGroup := range header.Links {
		mlinks := make([]*ui.Link, len(linkGroup.Links))
		for j, link := range linkGroup.Links {
			ariaLabel := fmt.Sprintf("%s in %s", link.Text, linkGroup.Name)
			mlinks[j] = ui.NewLink(link.Text, link.Url, ariaLabel)
		}
		links[i] = ui.LinkGroup{
			Name:  ui.NewLink(linkGroup.Name, "", ""),
			Links: mlinks,
		}
	}

	return &ui.ConsoleHeader{
		Oncalls:    oncalls,
		Links:      links,
		TreeStatus: treeStatus,
	}, nil
}

// consoleRenderer is a wrapper around Console to provide additional methods.
type consoleRenderer struct {
	*ui.Console
}

// ConsoleTable generates the main console table html.
//
// This cannot be generated with templates due to the 'recursive' nature of
// this layout.
func (c consoleRenderer) ConsoleTable() template.HTML {
	var buffer bytes.Buffer
	// The first node is a dummy node
	for _, column := range c.Table.Children {
		column.RenderHTML(&buffer, 1, c.MaxDepth)
	}
	return template.HTML(buffer.String())
}

// ConsoleSummary generates the html for the console's builders in the console list.
//
// It is similar to ConsoleTable, but flattens the structure.
func (c consoleRenderer) ConsoleSummary() template.HTML {
	var buffer bytes.Buffer
	for _, column := range c.Table.Children {
		column.RenderHTML(&buffer, 1, -1)
	}
	return template.HTML(buffer.String())
}

func (c consoleRenderer) BuilderLink(bs *model.BuildSummary) (*ui.Link, error) {
	_, _, builderName, err := buildsource.BuilderID(bs.BuilderID).Split()
	if err != nil {
		return nil, err
	}
	return ui.NewLink(builderName, "/"+bs.BuilderID, fmt.Sprintf("builder %s", builderName)), nil
}

// ConsoleHandler renders the console page.
func ConsoleHandler(c *router.Context) {
	project := c.Params.ByName("project")
	if project == "" {
		ErrorHandler(c, errors.New("Missing Project", common.CodeParameterError))
		return
	}
	group := c.Params.ByName("group")

	// If group is a tryserver group, redirect to builders view.
	if strings.Contains(group, "tryserver") {
		redirect("/p/:project/g/:group/builders", http.StatusFound)(c)
		return
	}

	const defaultLimit = 50
	const maxLimit = 1000
	limit := defaultLimit
	if tLimit := GetLimit(c.Request, -1); tLimit >= 0 {
		limit = tLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	result, err := console(c.Context, project, group, limit)
	if err != nil {
		ErrorHandler(c, err)
		return
	}

	var reload *int
	if tReload := GetReload(c.Request, -1); tReload >= 0 {
		reload = &tReload
	}

	templates.MustRender(c.Context, c.Writer, "pages/console.html", templates.Args{
		"Console": consoleRenderer{result},
		"Reload":  reload,
		"Navi":    ProjectLinks(project, group),
	})
}

// ProjectLink returns the navigation list surrounding a project and optionally group.
func ProjectLinks(project, group string) []ui.LinkGroup {
	projLinks := []*ui.Link{
		ui.NewLink(
			"Builders",
			fmt.Sprintf("/p/%s/builders", project),
			fmt.Sprintf("All builders for project %s", project))}
	links := []ui.LinkGroup{
		{
			Name: ui.NewLink(
				project,
				fmt.Sprintf("/p/%s", project),
				fmt.Sprintf("Project page for %s", project)),
			Links: projLinks,
		},
	}
	if group != "" {
		groupLinks := []*ui.Link{
			ui.NewLink(
				"Console",
				fmt.Sprintf("/p/%s/g/%s/console", project, group),
				fmt.Sprintf("Console for group %s in project %s", group, project)),
			ui.NewLink(
				"Builders",
				fmt.Sprintf("/p/%s/g/%s/builders", project, group),
				fmt.Sprintf("Builders for group %s in project %s", group, project)),
		}
		links = append(links, ui.LinkGroup{
			Name:  ui.NewLink(group, "", ""),
			Links: groupLinks,
		})
	}
	return links
}

// ConsolesHandler is responsible for taking a project name and rendering the
// console list page (defined in ./appengine/templates/pages/consoles.html).
func ConsolesHandler(c *router.Context, projectName string) {
	cons, err := common.GetProjectConsoles(c.Context, projectName)
	if err != nil {
		ErrorHandler(c, err)
		return
	}
	type fullConsole struct {
		ProjectID string
		Def       *config.Console
		Render    consoleRenderer
	}
	var consoles []fullConsole
	summaryMap, err := buildsource.GetConsoleSummariesFromDefs(c.Context, cons)
	if err != nil {
		ErrorHandler(c, err)
		return
	}
	for _, con := range cons {
		summary, ok := summaryMap[con.ConsoleID()]
		if !ok {
			logging.Errorf(c.Context, "console summary for %s not found", con.ConsoleID())
			continue
		}
		respConsole, err := consolePreview(c.Context, summary, &con.Def)
		if err != nil {
			logging.WithError(err).Errorf(c.Context, "failed to generate resp console")
			continue
		}
		full := fullConsole{
			ProjectID: con.ProjectID(),
			Def:       &con.Def,
			Render:    consoleRenderer{respConsole},
		}
		consoles = append(consoles, full)
	}

	var reload *int
	if tReload := GetReload(c.Request, -1); tReload >= 0 {
		reload = &tReload
	}

	templates.MustRender(c.Context, c.Writer, "pages/consoles.html", templates.Args{
		"ProjectName": projectName,
		"Consoles":    consoles,
		"Reload":      reload,
		"Navi":        ProjectLinks(projectName, ""),
	})
}

func consoleTestData() []common.TestBundle {
	builder := &ui.BuilderRef{
		Name:      "tester",
		ShortName: "tst",
		Build: []*model.BuildSummary{
			{
				Summary: model.Summary{
					Status: model.Success,
				},
			},
			nil,
		},
		Builder: &model.BuilderSummary{
			BuilderID:          "tester",
			LastFinishedStatus: model.InfraFailure,
		},
	}
	root := ui.NewCategory("Root")
	root.AddBuilder([]string{"cat1", "cat2"}, builder)
	return []common.TestBundle{
		{
			Description: "Full console with Header",
			Data: templates.Args{
				"Navi": ProjectLinks("Testing", "Test"),
				"Console": consoleRenderer{&ui.Console{
					Name:    "Test",
					Project: "Testing",
					Header: &ui.ConsoleHeader{
						Oncalls: []ui.Oncall{
							{
								Name: "Sheriff",
								Emails: []string{
									"test@example.com",
									"watcher@example.com",
								},
							},
						},
						Links: []ui.LinkGroup{
							{
								Name: ui.NewLink("Some group", "", ""),
								Links: []*ui.Link{
									ui.NewLink("LiNk", "something", ""),
									ui.NewLink("LiNk2", "something2", ""),
								},
							},
						},
						ConsoleGroups: []ui.ConsoleGroup{
							{
								Title: ui.NewLink("bah", "something2", ""),
								Consoles: []ui.ConsoleSummary{
									{
										Name: ui.NewLink("hurrah", "something2", ""),
										Builders: []*model.BuilderSummary{
											{
												LastFinishedStatus: model.Success,
											},
											{
												LastFinishedStatus: model.Success,
											},
											{
												LastFinishedStatus: model.Failure,
											},
										},
									},
								},
							},
							{
								Consoles: []ui.ConsoleSummary{
									{
										Name: ui.NewLink("hurrah", "something2", ""),
										Builders: []*model.BuilderSummary{
											{
												LastFinishedStatus: model.Success,
											},
										},
									},
								},
							},
						},
					},
					Commit: []ui.Commit{
						{
							AuthorEmail: "x@example.com",
							CommitTime:  time.Date(12, 12, 12, 12, 12, 12, 0, time.UTC),
							Revision:    ui.NewLink("12031802913871659324", "blah blah blah", ""),
							Description: "Me too.",
						},
						{
							AuthorEmail: "y@example.com",
							CommitTime:  time.Date(12, 12, 12, 12, 12, 11, 0, time.UTC),
							Revision:    ui.NewLink("120931820931802913", "blah blah blah 1", ""),
							Description: "I did something.",
						},
					},
					Table:    *root,
					MaxDepth: 3,
				}},
			},
		},
	}
}
