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

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/buildbucket/bbperms"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/caching/layered"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"
	tspb "go.chromium.org/luci/tree_status/proto/v1"

	"go.chromium.org/luci/milo/frontend/handlers/ui"
	"go.chromium.org/luci/milo/internal/buildsource"
	"go.chromium.org/luci/milo/internal/config"
	"go.chromium.org/luci/milo/internal/git"
	"go.chromium.org/luci/milo/internal/model"
	"go.chromium.org/luci/milo/internal/projectconfig"
	"go.chromium.org/luci/milo/internal/utils"
	projectconfigpb "go.chromium.org/luci/milo/proto/projectconfig"
)

func logTimer(c context.Context, message string) func() {
	tStart := clock.Now(c)
	return func() {
		logging.Debugf(c, "%s: took %s", message, clock.Since(c, tStart))
	}
}

// validateFaviconURL checks to see if the URL is well-formed and from an allowed host.
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

func getFaviconURL(c context.Context, def *projectconfigpb.Console) string {
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

func buildTreeFromDef(def *projectconfigpb.Console, getColumnSummaries columnSummaryFn) (*ui.Category, int) {
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
func getConsoleGroups(def *projectconfigpb.Header, summaries map[projectconfig.ConsoleID]*ui.BuilderSummaryGroup) []ui.ConsoleGroup {
	if def == nil || len(def.GetConsoleGroups()) == 0 {
		// No header, no console groups.
		return nil
	}
	groups := def.GetConsoleGroups()
	consoleGroups := make([]ui.ConsoleGroup, len(groups))
	for i, group := range groups {
		groupSummaries := make([]*ui.BuilderSummaryGroup, len(group.ConsoleIds))
		for j, id := range group.ConsoleIds {
			cid, err := projectconfig.ParseConsoleID(id)
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

func consoleRowCommits(c context.Context, project string, def *projectconfigpb.Console, limit int) (
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
			Tag(grpcutil.NotFoundTag).Err()
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
		ct := commit.Committer.Time.AsTime()
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

func console(c context.Context, project, id string, limit int, con *projectconfig.Console, headerCons []*projectconfig.Console, consoleGroupsErr error) (*ui.Console, error) {
	def := con.Def
	consoleID := projectconfig.ConsoleID{Project: project, ID: id}
	var header *ui.ConsoleHeader
	var rows []*buildsource.ConsoleRow
	var commits []ui.Commit
	var builderSummaries map[projectconfig.ConsoleID]*ui.BuilderSummaryGroup
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
		if consoleGroupsErr == nil {
			header.ConsoleGroups = getConsoleGroups(def.Header, builderSummaries)
		} else {
			header.ConsoleGroupsErr = consoleGroupsErr
		}
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

var treeStatusCache = layered.RegisterCache(layered.Parameters[*ui.TreeStatus]{
	ProcessCacheCapacity: 256,
	GlobalNamespace:      "tree-status",
	Marshal: func(item *ui.TreeStatus) ([]byte, error) {
		return json.Marshal(item)
	},
	Unmarshal: func(blob []byte) (*ui.TreeStatus, error) {
		treeStatus := &ui.TreeStatus{}
		err := json.Unmarshal(blob, treeStatus)
		return treeStatus, err
	},
})

// getTreeStatus returns the current tree status from either the LUCI Tree Status
// service or the chromium-status app.
// This never errors, instead it constructs a fake purple TreeStatus
// Host is the url of the soon-to-be-deprecated chromium-status host.
// Name is the name of the tree in the LUCI Tree Status service.
// If both name and host are provided, host will be ignored.
func getTreeStatus(c context.Context, host string, name string) *ui.TreeStatus {
	if name != "" {
		url := &url.URL{
			Path: fmt.Sprintf("/ui/tree-status/%s", name),
		}
		status, err := fetchLUCITreeStatus(c, name, url)
		if err != nil {
			// Generate a fake tree status.
			logging.WithError(err).Errorf(c, "loading tree status")
			status = &ui.TreeStatus{
				GeneralState: "maintenance",
				Message:      "could not load tree status",
				URL:          url,
			}
		}
		return status
	}
	// TODO: Delete this after migrating configs.
	q := url.Values{}
	q.Add("format", "json")
	url := (&url.URL{
		Scheme:   "https",
		Host:     host,
		Path:     "current",
		RawQuery: q.Encode(),
	}).String()
	status, err := treeStatusCache.GetOrCreate(c, url, func() (v *ui.TreeStatus, exp time.Duration, err error) {
		out := &ui.TreeStatus{}
		if err := utils.GetJSONData(http.DefaultClient, url, out); err != nil {
			return nil, 0, err
		}
		return out, 30 * time.Second, nil
	})

	if err != nil {
		// Generate a fake tree status.
		logging.WithError(err).Errorf(c, "loading tree status")
		status = &ui.TreeStatus{
			GeneralState: "maintenance",
			Message:      "could not load tree status",
		}
	}

	return status
}

func fetchLUCITreeStatus(c context.Context, name string, url *url.URL) (*ui.TreeStatus, error) {
	luciTreeStatusHost, err := luciTreeStatusHost(c)
	if err != nil {
		return nil, errors.Annotate(err, "getting luci tree status host").Err()
	}
	client, err := NewTreeStatusClient(c, luciTreeStatusHost)
	if err != nil {
		return nil, errors.Annotate(err, "creating tree status client").Err()
	}
	response, err := client.GetStatus(c, &tspb.GetStatusRequest{Name: fmt.Sprintf("trees/%s/status/latest", name)})
	if err != nil {
		return nil, errors.Annotate(err, "calling GetStatus").Err()
	}
	out := &ui.TreeStatus{
		Username:        response.CreateUser,
		GeneralState:    treeStatusState(response.GeneralState),
		Date:            response.CreateTime.AsTime().Format("2006-01-02T15:04:05.999999"),
		Message:         response.Message,
		CanCommitFreely: response.GeneralState == tspb.GeneralState_OPEN,
		URL:             url,
	}
	return out, nil
}

func treeStatusState(state tspb.GeneralState) ui.TreeStatusState {
	switch state {
	case tspb.GeneralState_OPEN:
		return ui.TreeStatusState("open")
	case tspb.GeneralState_CLOSED:
		return ui.TreeStatusState("closed")
	case tspb.GeneralState_THROTTLED:
		return ui.TreeStatusState("throttled")
	default:
		return ui.TreeStatusState("maintenance")
	}
}

func luciTreeStatusHost(c context.Context) (string, error) {
	settings := config.GetSettings(c)
	if settings.LuciTreeStatus == nil || settings.LuciTreeStatus.Host == "" {
		return "", errors.New("missing luci tree status host in settings")
	}
	return settings.LuciTreeStatus.Host, nil
}

func NewTreeStatusClient(ctx context.Context, luciTreeStatusHost string) (tspb.TreeStatusClient, error) {
	t, err := auth.GetRPCTransport(ctx, auth.AsSessionUser)
	if err != nil {
		return nil, err
	}
	rpcOpts := prpc.DefaultOptions()
	rpcOpts.Insecure = lhttp.IsLocalHost(luciTreeStatusHost)
	prpcClient := &prpc.Client{
		C:       &http.Client{Transport: t},
		Host:    luciTreeStatusHost,
		Options: rpcOpts,
	}

	return tspb.NewTreeStatusPRPCClient(prpcClient), nil
}

var oncallDataCache = layered.RegisterCache(layered.Parameters[*ui.Oncall]{
	ProcessCacheCapacity: 256,
	GlobalNamespace:      "oncall-data",
	Marshal: func(item *ui.Oncall) ([]byte, error) {
		return json.Marshal(item)
	},
	Unmarshal: func(blob []byte) (*ui.Oncall, error) {
		oncall := &ui.Oncall{}
		err := json.Unmarshal(blob, oncall)
		return oncall, err
	},
})

// getOncallData fetches oncall data and caches it for 10 minutes.
func getOncallData(c context.Context, config *projectconfigpb.Oncall) (*ui.OncallSummary, error) {
	oncall, err := oncallDataCache.GetOrCreate(c, config.Url, func() (v *ui.Oncall, exp time.Duration, err error) {
		out := &ui.Oncall{}
		if err := utils.GetJSONData(http.DefaultClient, config.Url, out); err != nil {
			return nil, 0, err
		}
		return out, 10 * time.Minute, nil
	})

	var renderedHTML template.HTML
	if err == nil {
		renderedHTML = renderOncallers(config, oncall)
	} else {
		renderedHTML = template.HTML("ERROR: Fetching oncall failed")
	}
	return &ui.OncallSummary{
		Name:      config.Name,
		Oncallers: renderedHTML,
	}, nil
}

// renderOncallers renders a summary string to be displayed in the UI, showing
// the current oncallers.
func renderOncallers(config *projectconfigpb.Oncall, jsonResult *ui.Oncall) template.HTML {
	var oncallers string
	if len(jsonResult.Emails) == 1 {
		oncallers = jsonResult.Emails[0]
	} else if len(jsonResult.Emails) > 1 {
		if config.ShowPrimarySecondaryLabels {
			var sb strings.Builder
			fmt.Fprintf(&sb, "%v (primary)", jsonResult.Emails[0])
			for _, oncaller := range jsonResult.Emails[1:] {
				fmt.Fprintf(&sb, ", %v (secondary)", oncaller)
			}
			oncallers = sb.String()
		} else {
			oncallers = strings.Join(jsonResult.Emails, ", ")
		}
	} else if jsonResult.Primary != "" {
		if len(jsonResult.Secondaries) > 0 {
			var sb strings.Builder
			fmt.Fprintf(&sb, "%v (primary)", jsonResult.Primary)
			for _, oncaller := range jsonResult.Secondaries {
				fmt.Fprintf(&sb, ", %v (secondary)", oncaller)
			}
			oncallers = sb.String()
		} else {
			oncallers = jsonResult.Primary
		}
	} else {
		oncallers = "<none>"
	}
	return utils.ObfuscateEmail(utils.ShortenEmail(oncallers))
}

func consoleHeaderOncall(c context.Context, config []*projectconfigpb.Oncall) ([]*ui.OncallSummary, error) {
	// Get oncall data from URLs.
	oncalls := make([]*ui.OncallSummary, len(config))
	err := parallel.WorkPool(8, func(ch chan<- func() error) {
		for i, oc := range config {
			i := i
			oc := oc
			ch <- func() (err error) {
				oncalls[i], err = getOncallData(c, oc)
				return
			}
		}
	})
	return oncalls, err
}

func consoleHeader(c context.Context, project string, header *projectconfigpb.Header) (*ui.ConsoleHeader, error) {
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

	var oncalls []*ui.OncallSummary
	var treeStatus *ui.TreeStatus
	// Get the oncall and tree status concurrently.
	if err := parallel.FanOutIn(func(ch chan<- func() error) {
		ch <- func() (err error) {
			// Hide the oncall section to external users.
			// TODO(weiweilin): Once the upstream service (rotation proxy) supports
			// ACL checks, we should use the user's credential to query the oncall
			// data and display it.
			user := auth.CurrentUser(c)
			if !strings.HasSuffix(user.Email, "@google.com") {
				return nil
			}

			oncalls, err = consoleHeaderOncall(c, header.Oncalls)
			if err != nil {
				logging.WithError(err).Errorf(c, "getting oncalls")
			}
			return nil
		}
		if header.TreeStatusHost != "" || header.TreeName != "" {
			ch <- func() error {
				treeStatus = getTreeStatus(c, header.TreeStatusHost, header.TreeName)
				// TODO (nqmtuan): Remove this after we move everything to tree name.
				if header.TreeName == "" {
					treeStatus.URL = &url.URL{Scheme: "https", Host: header.TreeStatusHost}
				}
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
func consoleHeaderGroupIDs(project string, config []*projectconfigpb.ConsoleSummaryGroup) ([]projectconfig.ConsoleID, error) {
	consoleIDSet := map[projectconfig.ConsoleID]struct{}{}
	for _, group := range config {
		for _, id := range group.ConsoleIds {
			cid, err := projectconfig.ParseConsoleID(id)
			if err != nil {
				return nil, err
			}
			consoleIDSet[cid] = struct{}{}
		}
	}
	consoleIDs := make([]projectconfig.ConsoleID, 0, len(consoleIDSet))
	for cid := range consoleIDSet {
		consoleIDs = append(consoleIDs, cid)
	}
	return consoleIDs, nil
}

// filterUnauthorizedBuildersFromConsoles filters out builders the user does not have access to.
func filterUnauthorizedBuildersFromConsoles(c context.Context, cons []*projectconfig.Console) error {
	allRealms := stringset.New(0)
	for _, con := range cons {
		allRealms = allRealms.Union(con.BuilderRealms())
	}

	allowedRealms := stringset.New(0)
	for realm := range allRealms {
		allowed, err := auth.HasPermission(c, bbperms.BuildsList, realm, nil)
		if err != nil {
			return err
		}
		if allowed {
			allowedRealms.Add(realm)
		}
	}

	for _, con := range cons {
		con.FilterBuilders(allowedRealms)
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
	con, err := projectconfig.GetConsole(c.Request.Context(), project, group)
	switch {
	case err != nil:
		return err
	case con.IsExternal():
		// We don't allow navigating directly to external consoles.
		return projectconfig.ErrConsoleNotFound
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

	var headerCons []*projectconfig.Console
	var headerConsError error
	if con.Def.Header != nil {
		ids, err := consoleHeaderGroupIDs(project, con.Def.Header.GetConsoleGroups())
		if err != nil {
			return err
		}
		headerCons, err = projectconfig.GetConsoles(c.Request.Context(), ids)
		if err != nil {
			headerConsError = errors.Annotate(err, "error getting header consoles").Err()
			headerCons = make([]*projectconfig.Console, 0)
		}
	}
	if err := filterUnauthorizedBuildersFromConsoles(c.Request.Context(), append(headerCons, con)); err != nil {
		return errors.Annotate(err, "error authorizing user").Err()
	}

	// Process the request and generate a renderable structure.
	result, err := console(c.Request.Context(), project, group, limit, con, headerCons, headerConsError)
	if err != nil {
		return err
	}

	templates.MustRender(c.Request.Context(), c.Writer, "pages/console.html", templates.Args{
		"Console": consoleRenderer{result},
		"Expand":  con.Def.DefaultExpand,
	})
	return nil
}
