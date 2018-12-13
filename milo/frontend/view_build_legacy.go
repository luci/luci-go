// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package frontend

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	buildbotapi "go.chromium.org/luci/milo/api/buildbot"
	"go.chromium.org/luci/milo/buildsource/buildbot"
	"go.chromium.org/luci/milo/buildsource/buildbot/buildstore"
	"go.chromium.org/luci/milo/buildsource/rawpresentation"
	"go.chromium.org/luci/milo/buildsource/swarming"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/milo/frontend/ui"
)

// handleBuildbotBuild renders a buildbot build.
// Requires emulationMiddleware.
func handleBuildbotBuild(c *router.Context) error {
	buildNum, err := strconv.Atoi(c.Params.ByName("number"))
	if err != nil {
		return errors.Annotate(err, "build number is not a number").
			Tag(common.CodeParameterError).
			Err()
	}
	id := buildbotapi.BuildID{
		Master:  c.Params.ByName("master"),
		Builder: c.Params.ByName("builder"),
		Number:  buildNum,
	}
	if err := id.Validate(); err != nil {
		return err
	}

	// If this build is emulated, redirect to LUCI.
	b, err := buildstore.EmulationOf(c.Context, id)
	switch {
	case err != nil:
		return err
	case b != nil && b.Number != nil:
		u := *c.Request.URL
		u.Path = fmt.Sprintf("/p/%s/builders/%s/%s/%d", b.Project, b.Bucket, b.Builder, *b.Number)
		http.Redirect(c.Writer, c.Request, u.String(), http.StatusFound)
		return nil
	default:
		build, err := buildbot.GetBuild(c.Context, id)
		return renderBuildLegacy(c, build, false, err)
	}
}

func handleSwarmingBuild(c *router.Context) error {
	build, err := swarming.GetBuild(
		c.Context,
		c.Request.FormValue("server"),
		c.Params.ByName("id"))
	return renderBuildLegacy(c, build, false, err)
}

func handleRawPresentationBuild(c *router.Context) error {
	build, err := rawpresentation.GetBuild(
		c.Context,
		c.Params.ByName("logdog_host"),
		types.ProjectName(c.Params.ByName("project")),
		types.StreamPath(strings.Trim(c.Params.ByName("path"), "/")))
	return renderBuildLegacy(c, build, false, err)
}

func getStepDisplayPrefCookie(c *router.Context) ui.StepDisplayPref {
	switch cookie, err := c.Request.Cookie("stepDisplayPref"); err {
	case nil:
		return ui.StepDisplayPref(cookie.Value)
	case http.ErrNoCookie:
		return ui.StepDisplayDefault
	default:
		logging.WithError(err).Errorf(c.Context, "failed to read stepDisplayPref cookie")
		return ui.StepDisplayDefault
	}
}

// renderBuildLegacy is a shortcut for rendering build or returning err if it is not
// nil. Also calls build.Fix().
func renderBuildLegacy(c *router.Context, build *ui.MiloBuildLegacy, renderTimeline bool, err error) error {
	if err != nil {
		return err
	}

	build.StepDisplayPref = getStepDisplayPrefCookie(c)
	build.Fix(c.Context)

	timelineJSON := ""
	if renderTimeline {
		if timelineJSON, err = timelineData(build); err != nil {
			return err
		}
	}

	templates.MustRender(c.Context, c.Writer, "pages/build_legacy.html", templates.Args{
		"Build":        build,
		"TimelineJSON": timelineJSON,
	})
	return nil
}

// timelineData returns the timelineJSON for a vis timeline timeline
// as a JSON.parse parseable string that will contain the necessary
// Groups and Items.
func timelineData(build *ui.MiloBuildLegacy) (string, error) {
	// stepData is extra data to deliver with the groups and items (see below) for the
	// Javascript vis Timeline component.
	type stepData struct {
		Label           string       `json:"label"`
		Text            []string     `json:"text"`
		Duration        string       `json:"duration"`
		MainLink        ui.LinkSet   `json:"mainLink"`
		SubLink         []ui.LinkSet `json:"subLink"`
		StatusClassName string       `json:"statusClassName"`
	}

	// group corresponds to, and matches the shape of, a Group for the Javascript
	// vis Timeline component http://visjs.org/docs/timeline/#groups. Data
	// rides along as an extra property (unused by vis Timeline itself) used
	// in client side rendering. Each Group is rendered as its own row in the
	// timeline component on to which Items are rendered. Currently we only render
	// one Item per Group, that is one thing per row.
	type group struct {
		ID   string   `json:"id"`
		Data stepData `json:"data"`
	}

	// item corresponds to, and matches the shape of, an Item for the Javascript
	// vis Timeline component http://visjs.org/docs/timeline/#items. Data
	// rides along as an extra property (unused by vis Timeline itself) used
	// in client side rendering. Each Item is rendered to a Group which corresponds
	// to a row. Currently we only render one Item per Group, that is one thing per
	// row.
	type item struct {
		ID        string   `json:"id"`
		Group     string   `json:"group"`
		Start     int64    `json:"start"`
		End       int64    `json:"end"`
		Type      string   `json:"type"`
		ClassName string   `json:"className"`
		Data      stepData `json:"data"`
	}

	groups := make([]group, len(build.Components))
	items := make([]item, len(build.Components))
	for index, comp := range build.Components {
		groupID := strconv.Itoa(index)
		statusClassName := fmt.Sprintf("status-%s", comp.Status)
		data := stepData{
			Label:           html.EscapeString(comp.Label.Label),
			Text:            sanitize(comp.TextBR()),
			Duration:        humanDuration(comp.ExecutionTime.Duration),
			MainLink:        sanitizeLinkSet(comp.MainLink),
			SubLink:         sanitizeLinkSets(comp.SubLink),
			StatusClassName: statusClassName,
		}
		groups[index] = group{groupID, data}
		items[index] = item{
			ID:        groupID,
			Group:     groupID,
			Start:     milliseconds(comp.ExecutionTime.Started),
			End:       milliseconds(comp.ExecutionTime.Finished),
			Type:      "range",
			ClassName: statusClassName,
			Data:      data,
		}
	}

	timeline, err := json.Marshal(map[string]interface{}{
		"groups": groups,
		"items":  items,
	})
	if err != nil {
		return "", err
	}

	return string(timeline), nil
}

func sanitize(values []string) []string {
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = html.EscapeString(value)
	}
	return result
}

func sanitizeLinkSet(linkSet ui.LinkSet) ui.LinkSet {
	result := make(ui.LinkSet, len(linkSet))
	for i, link := range linkSet {
		result[i] = &ui.Link{
			Link: model.Link{
				Label: html.EscapeString(link.Label),
				URL:   html.EscapeString(link.URL),
			},
			AriaLabel: html.EscapeString(link.AriaLabel),
			Img:       html.EscapeString(link.Img),
			Alt:       html.EscapeString(link.Alt),
		}
	}
	return result
}

func sanitizeLinkSets(linkSets []ui.LinkSet) []ui.LinkSet {
	result := make([]ui.LinkSet, len(linkSets))
	for i, linkSet := range linkSets {
		result[i] = sanitizeLinkSet(linkSet)
	}
	return result
}

func milliseconds(time time.Time) int64 {
	return time.UnixNano() / 1e6
}
