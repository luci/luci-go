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
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes"
	"google.golang.org/grpc/codes"

	"go.chromium.org/gae/service/memcache"
	"go.chromium.org/gae/service/urlfetch"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/common/api/gitiles"
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
	if def.FaviconUrl == "" {
		return ""
	}
	if err := validateFaviconURL(def.FaviconUrl); err != nil {
		logging.WithError(err).Warningf(c, "invalid favicon URL")
		return ""
	}
	return def.FaviconUrl
}

// columnSummaryFn is called by buildTreeFromDef.
//
// columnIdx is the index of the current column we're processing. This
// corresponds to one of the Builder messages in the Console config proto
// message.
//
// This function should return a BuilderSummary and []BuildSummary for the
// specified console column.
type columnSummaryFn func(columnIdx int) (*model.BuilderSummary, []*model.BuildSummary)

func buildTreeFromDef(def *config.Console, getColumnSummaries columnSummaryFn) (*ui.Category, int) {
	// Build console table tree from builders.
	categoryTree := ui.NewCategory("")
	depth := 0
	for columnIdx, builderMsg := range def.Builders {
		ref := &ui.BuilderRef{
			ShortName: builderMsg.ShortName,
		}
		ref.Builder, ref.Build = getColumnSummaries(columnIdx)
		if ref.Builder != nil {
			// TODO(iannucci): This is redundant; use Builder.BuilderID directly.
			ref.ID = ref.Builder.BuilderID
		}
		categories := builderMsg.ParseCategory()
		if len(categories) > depth {
			depth = len(categories)
		}
		categoryTree.AddBuilder(categories, ref)
	}
	return categoryTree, depth
}

// getConsoleGroups extracts the console summaries for all header summaries
// out of the summaries map into console groups for the header.
func getConsoleGroups(def *config.Header, summaries map[common.ConsoleID]*ui.BuilderSummaryGroup) []ui.ConsoleGroup {
	if def == nil || len(def.GetConsoleGroups()) == 0 {
		// No header, no console groups.
		return nil
	}
	groups := def.GetConsoleGroups()
	consoleGroups := make([]ui.ConsoleGroup, len(groups))
	for i, group := range groups {
		groupSummaries := make([]*ui.BuilderSummaryGroup, len(group.ConsoleIds))
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
	repoHost, repoProject, err := gitiles.ParseRepoURL(def.RepoUrl)
	if err != nil {
		return nil, nil, errors.Annotate(err, "invalid repo URL %q in the config", def.RepoUrl).Err()
	}
	rawCommits, err := git.Get(c).CombinedLogs(c, repoHost, repoProject, def.ExcludeRef, def.Refs, limit)
	switch grpcutil.Code(err) {
	case codes.OK:
		// Do nothing, all is good.
	case codes.NotFound:
		return nil, nil, errors.Reason("incorrect repo URL %q in the config or no access", def.RepoUrl).
			Tag(grpcutil.InternalTag).Err()
	default:
		return nil, nil, err
	}
	tGitiles()

	commitIDs := make([]string, len(rawCommits))
	for i, c := range rawCommits {
		commitIDs[i] = c.Id
	}

	tBuilds := logTimer(c, "Rows: loading builds")
	rows, err := buildsource.GetConsoleRows(c, project, def, commitIDs)
	tBuilds()
	if err != nil {
		return nil, nil, err
	}

	// Build list of commits.
	commits := make([]ui.Commit, len(rawCommits))
	for row, commit := range rawCommits {
		ct, _ := ptypes.Timestamp(commit.Committer.Time)
		commits[row] = ui.Commit{
			AuthorName:  commit.Author.Name,
			AuthorEmail: commit.Author.Email,
			CommitTime:  ct,
			Repo:        def.RepoUrl,
			Description: commit.Message,
			Revision: ui.NewLink(
				commit.Id,
				def.RepoUrl+"/+/"+commit.Id,
				fmt.Sprintf("commit by %s", commit.Author.Email)),
		}
	}

	return rows, commits, nil
}

func console(c context.Context, project, id string, limit int, con *common.Console, headerCons []*common.Console) (*ui.Console, error) {
	def := &con.Def
	consoleID := common.ConsoleID{Project: project, ID: id}
	var header *ui.ConsoleHeader
	var rows []*buildsource.ConsoleRow
	var commits []ui.Commit
	var builderSummaries map[common.ConsoleID]*ui.BuilderSummaryGroup
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
			builderSummaries, err = buildsource.GetConsoleSummariesFromDefs(c, append(headerCons, con), project)
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
	if header != nil {
		header.ConsoleGroups = getConsoleGroups(def.Header, builderSummaries)
	}

	// Reassemble the builder summaries and rows into the categoryTree.
	categoryTree, depth := buildTreeFromDef(def, func(columnIdx int) (*model.BuilderSummary, []*model.BuildSummary) {
		builds := make([]*model.BuildSummary, len(commits))
		for row := range commits {
			if summaries := rows[row].Builds[columnIdx]; len(summaries) > 0 {
				builds[row] = summaries[0]
			}
		}
		return builderSummaries[consoleID].Builders[columnIdx], builds
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

func consolePreview(c context.Context, summaries *ui.BuilderSummaryGroup, def *config.Console) (*ui.Console, error) {
	categoryTree, depth := buildTreeFromDef(def, func(columnIdx int) (*model.BuilderSummary, []*model.BuildSummary) {
		return summaries.Builders[columnIdx], nil
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

func consoleHeader(c context.Context, project string, header *config.Header) (*ui.ConsoleHeader, error) {
	// Return nil if the header is empty.
	switch {
	case len(header.Oncalls) != 0:
		// continue
	case len(header.Links) != 0:
		// continue
	case header.TreeStatusHost != "":
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
	for _, column := range c.Table.Children() {
		column.RenderHTML(&buffer, 1, c.MaxDepth)
	}
	return template.HTML(buffer.String())
}

// ConsoleSummary generates the html for the console's builders in the console list.
//
// It is similar to ConsoleTable, but flattens the structure.
func (c consoleRenderer) ConsoleSummary() template.HTML {
	var buffer bytes.Buffer
	for _, column := range c.Table.Children() {
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

// consoleHeaderGroupIDs extracts the console group IDs out of the header config.
func consoleHeaderGroupIDs(project string, config []*config.ConsoleSummaryGroup) ([]common.ConsoleID, error) {
	consoleIDSet := map[common.ConsoleID]struct{}{}
	for _, group := range config {
		for _, id := range group.ConsoleIds {
			cid, err := common.ParseConsoleID(id)
			if err != nil {
				return nil, err
			}
			consoleIDSet[cid] = struct{}{}
		}
	}
	consoleIDs := make([]common.ConsoleID, 0, len(consoleIDSet))
	for cid := range consoleIDSet {
		consoleIDs = append(consoleIDs, cid)
	}
	return consoleIDs, nil
}

// filterUnauthorizedBuildersFromConsoles filters out builders the user does not have access to.
func filterUnauthorizedBuildersFromConsoles(c context.Context, cons []*common.Console) error {
	buckets := stringset.New(0)
	for _, con := range cons {
		buckets = buckets.Union(con.Buckets())
	}
	perms, err := common.BucketPermissions(c, buckets.ToSlice()...)
	if err != nil {
		return err
	}
	for _, con := range cons {
		con.FilterBuilders(perms)
	}
	return nil
}

// ConsoleHandler renders the console page.
func ConsoleHandler(c *router.Context) error {
	project := c.Params.ByName("project")
	if project == "" {
		return errors.New("missing project", grpcutil.InvalidArgumentTag)
	}
	group := c.Params.ByName("group")

	// Get console from datastore and filter out builders from the definition.
	con, err := common.GetConsole(c.Context, project, group)
	switch {
	case err != nil:
		return err
	case con.Def.BuilderViewOnly:
		redirect("/p/:project/g/:group/builders", http.StatusFound)(c)
		return nil
	}

	defaultLimit := 50
	if con.Def.DefaultCommitLimit > 0 {
		defaultLimit = int(con.Def.DefaultCommitLimit)
	}
	const maxLimit = 1000
	limit := defaultLimit
	if tLimit := GetLimit(c.Request, -1); tLimit >= 0 {
		limit = tLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	var headerCons []*common.Console
	if con.Def.Header != nil {
		ids, err := consoleHeaderGroupIDs(project, con.Def.Header.GetConsoleGroups())
		if err != nil {
			return err
		}
		headerCons, err = common.GetConsoles(c.Context, ids)
		if err != nil {
			return errors.Annotate(err, "error getting header consoles").Err()
		}
	}
	if err := filterUnauthorizedBuildersFromConsoles(c.Context, append(headerCons, con)); err != nil {
		return errors.Annotate(err, "error authorizing user").Err()
	}

	// Process the request and generate a renderable structure.
	result, err := console(c.Context, project, group, limit, con, headerCons)
	if err != nil {
		return err
	}

	templates.MustRender(c.Context, c.Writer, "pages/console.html", templates.Args{
		"Console": consoleRenderer{result},
		"Expand":  con.Def.DefaultExpand,
	})
	return nil
}

// ConsolesHandler is responsible for taking a project name and rendering the
// console list page (defined in ./appengine/templates/pages/builder_groups.html).
func ConsolesHandler(c *router.Context, projectID string) error {
	// Get consoles related to this project and filter out all builders.
	cons, err := common.GetProjectConsoles(c.Context, projectID)
	if err != nil {
		return err
	}
	if err := filterUnauthorizedBuildersFromConsoles(c.Context, cons); err != nil {
		return errors.Annotate(err, "error authorizing user").Err()
	}

	type fullConsole struct {
		ProjectID string
		Def       *config.Console
		Render    consoleRenderer
	}
	var consoles []fullConsole
	summaryMap, err := buildsource.GetConsoleSummariesFromDefs(c.Context, cons, projectID)
	if err != nil {
		return err
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

	templates.MustRender(c.Context, c.Writer, "pages/builder_groups.html", templates.Args{
		"ProjectID": projectID,
		"Consoles":  consoles,
	})
	return nil
}

func consoleTestData() []common.TestBundle {
	builder := &ui.BuilderRef{
		ID:        "buildbucket/luci.project-foo.try/builder-bar",
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
			BuilderID:          "buildbucket/luci.project-foo.try/builder-bar",
			ProjectID:          "project-foo",
			LastFinishedStatus: model.InfraFailure,
		},
	}
	root := ui.NewCategory("Root")
	root.AddBuilder([]string{"cat1", "cat2"}, builder)
	return []common.TestBundle{
		{
			Description: "Full console with Header",
			Data: templates.Args{
				"Expand": false,
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
								Consoles: []*ui.BuilderSummaryGroup{
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
								Consoles: []*ui.BuilderSummaryGroup{
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
