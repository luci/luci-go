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
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/info"

	"go.chromium.org/luci/common/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/analytics"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/milo/api/proto"
	"go.chromium.org/luci/milo/buildsource/buildbot/buildstore"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/frontend/ui"
)

// A collection of useful templating functions

// funcMap is what gets fed into the template bundle.
var funcMap = template.FuncMap{
	"faviconMIMEType": faviconMIMEType,
	"formatTime":      formatTime,
	"humanDuration":   humanDuration,
	"localTime":       localTime,
	"obfuscateEmail":  obfuscateEmail,
	"pagedURL":        pagedURL,
	"parseRFC3339":    parseRFC3339,
	"percent":         percent,
	"shortenEmail":    shortenEmail,
	"shortHash":       shortHash,
	"startswith":      strings.HasPrefix,
	"sub":             sub,
	"toLower":         strings.ToLower,
}

// localTime returns a <span> element with t in human format
// that will be converted to local timezone in the browser.
// Recommended usage: {{ .Date | localTime "N/A" }}
func localTime(ifZero string, t time.Time) template.HTML {
	if t.IsZero() {
		return template.HTML(template.HTMLEscapeString(ifZero))
	}
	milliseconds := t.UnixNano() / 1e6
	return template.HTML(fmt.Sprintf(
		`<span class="local-time" data-timestamp="%d">%s</span>`,
		milliseconds,
		t.Format(time.RFC850)))
}

// humanDuration translates d into a human readable string of x units y units,
// where x and y could be in days, hours, minutes, or seconds, whichever is the
// largest.
func humanDuration(d time.Duration) string {
	t := int64(d.Seconds())
	day := t / 86400
	hr := (t % 86400) / 3600

	if day > 0 {
		if hr != 0 {
			return fmt.Sprintf("%d days %d hrs", day, hr)
		}
		return fmt.Sprintf("%d days", day)
	}

	min := (t % 3600) / 60
	if hr > 0 {
		if min != 0 {
			return fmt.Sprintf("%d hrs %d mins", hr, min)
		}
		return fmt.Sprintf("%d hrs", hr)
	}

	sec := t % 60
	if min > 0 {
		if sec != 0 {
			return fmt.Sprintf("%d mins %d secs", min, sec)
		}
		return fmt.Sprintf("%d mins", min)
	}

	if sec != 0 {
		return fmt.Sprintf("%d secs", sec)
	}

	if d > time.Millisecond {
		return fmt.Sprintf("%d ms", d/time.Millisecond)
	}

	return "0"
}

// obfuscateEmail converts a string containing email adddress email@address.com
// into email<junk>@address.com.
func obfuscateEmail(email string) template.HTML {
	email = template.HTMLEscapeString(email)
	return template.HTML(strings.Replace(
		email, "@", "<span style=\"display:none\">ohnoyoudont</span>@", -1))
}

// parseRFC3339 parses time represented as a RFC3339 or RFC3339Nano string.
// If cannot parse, returns zero time.
func parseRFC3339(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return t
	}
	t, err = time.Parse(time.RFC3339Nano, s)
	if err == nil {
		return t
	}
	return time.Time{}
}

// formatTime takes a time object and returns a formatted RFC3339 string.
func formatTime(t time.Time) string {
	return t.Format(time.RFC3339)
}

// sub subtracts one number from another, because apparently go templates aren't
// smart enough to do that.
func sub(a, b int) int {
	return a - b
}

// shortenEmail shortens Google emails.
func shortenEmail(email string) string {
	return strings.Replace(email, "@google.com", "", -1)
}

// shortHash abbriviates a git hash into 6 characters.
func shortHash(s string) string {
	if len(s) > 6 {
		return s[0:6]
	}
	return s
}

// GetLimit extracts the "limit", "numbuilds", or "num_builds" http param from
// the request, or returns def implying no limit was specified.
func GetLimit(r *http.Request, def int) int {
	sLimit := r.FormValue("limit")
	if sLimit == "" {
		sLimit = r.FormValue("numbuilds")
		if sLimit == "" {
			sLimit = r.FormValue("num_builds")
			if sLimit == "" {
				return def
			}
		}
	}
	limit, err := strconv.Atoi(sLimit)
	if err != nil || limit < 0 {
		return def
	}
	return limit
}

// GetReload extracts the "reload" http param from the request,
// or returns def implying no limit was specified.
func GetReload(r *http.Request, def int) int {
	sReload := r.FormValue("reload")
	if sReload == "" {
		return def
	}
	refresh, err := strconv.Atoi(sReload)
	if err != nil || refresh < 0 {
		return def
	}
	return refresh
}

// pagedURL returns a self URL with the given cursor and limit paging options.
// if limit is set to 0, then inherit whatever limit is set in request.  If
// both are unspecified, then limit is omitted.
func pagedURL(r *http.Request, limit int, cursor string) string {
	if limit == 0 {
		limit = GetLimit(r, -1)
		if limit < 0 {
			limit = 0
		}
	}
	values := r.URL.Query()
	switch cursor {
	case "EMPTY":
		values.Del("cursor")
	case "":
		// Do nothing, just leave the cursor in.
	default:
		values.Set("cursor", cursor)
	}
	switch {
	case limit < 0:
		values.Del("limit")
	case limit > 0:
		values.Set("limit", fmt.Sprintf("%d", limit))
	}
	result := *r.URL
	result.RawQuery = values.Encode()
	return result.String()
}

// percent divides one number by a divisor and returns the percentage in string form.
func percent(numerator, divisor int) string {
	p := float64(numerator) * 100.0 / float64(divisor)
	return fmt.Sprintf("%.1f", p)
}

// faviconMIMEType derives the MIME type from a URL's file extension. Only valid
// favicon image formats are supported.
func faviconMIMEType(fileURL string) string {
	switch {
	case strings.HasSuffix(fileURL, ".png"):
		return "image/png"
	case strings.HasSuffix(fileURL, ".ico"):
		return "image/ico"
	case strings.HasSuffix(fileURL, ".jpeg"):
		fallthrough
	case strings.HasSuffix(fileURL, ".jpg"):
		return "image/jpeg"
	case strings.HasSuffix(fileURL, ".gif"):
		return "image/gif"
	}
	return ""
}

// getTemplateBundles is used to render HTML templates. It provides base args
// passed to all templates.  It takes a path to the template folder, relative
// to the path of the binary during runtime.
func getTemplateBundle(templatePath string) *templates.Bundle {
	return &templates.Bundle{
		Loader:          templates.FileSystemLoader(templatePath),
		DebugMode:       info.IsDevAppServer,
		DefaultTemplate: "base",
		DefaultArgs: func(c context.Context) (templates.Args, error) {
			rc := getRouterContext(c)
			path := rc.Request.URL.Path
			loginURL, err := auth.LoginURL(c, path)
			if err != nil {
				return nil, err
			}
			logoutURL, err := auth.LogoutURL(c, path)
			if err != nil {
				return nil, err
			}

			project := rc.Params.ByName("project")
			group := rc.Params.ByName("group")

			return templates.Args{
				"AppVersion":  strings.Split(info.VersionID(c), ".")[0],
				"IsAnonymous": auth.CurrentIdentity(c) == identity.AnonymousIdentity,
				"User":        auth.CurrentUser(c),
				"LoginURL":    loginURL,
				"LogoutURL":   logoutURL,
				"CurrentTime": clock.Now(c),
				"Analytics":   analytics.Snippet(c),
				"RequestID":   info.RequestID(c),
				"Request":     rc.Request,
				"Navi":        ProjectLinks(project, group),
				"ProjectID":   project,
			}, nil
		},
		FuncMap: funcMap,
	}
}

// A context key used to insert a *router.Context into
// a context.Context.
var routerContextKey = "router context"

// withRouterContext returns a context with the router.Context object
// in it.
func withRouterContext(c context.Context, r *router.Context) context.Context {
	return context.WithValue(c, &routerContextKey, r)
}

// withRouterContextMiddleware is a middleware that installs a router context
// into the context.
// This is used for various things in the default template.
func withRouterContextMiddleware(c *router.Context, next router.Handler) {
	c.Context = withRouterContext(c.Context, c)
	next(c)
}

func getRouterContext(c context.Context) *router.Context {
	if rc, ok := c.Value(&routerContextKey).(*router.Context); ok {
		return rc
	}
	panic("No http.request found in context")
}

// emulationMiddleware adds emulation options to the context.
// Expects c.Params to have master and builder parameters.
func emulationMiddleware(c *router.Context, next router.Handler) {
	opts := milo.EmulationOptions{Bucket: c.Request.FormValue("em-bucket")}
	if opts.Bucket != "" {
		if s := c.Request.FormValue("em-start"); s != "" {
			start, err := strconv.Atoi(s)
			if err != nil {
				ErrorHandler(c, errors.Annotate(err, "invalid emulation-start").Tag(common.CodeParameterError).Err())
				return
			}
			opts.StartFrom = int32(start)
		}
		c.Context = buildstore.WithEmulationOptions(
			c.Context,
			c.Params.ByName("master"),
			c.Params.ByName("builder"),
			opts)
	}

	next(c)
}

// ProjectLink returns the navigation list surrounding a project and optionally group.
func ProjectLinks(project, group string) []ui.LinkGroup {
	if project == "" {
		return nil
	}
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
