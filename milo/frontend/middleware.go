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
	"fmt"
	"html/template"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/info"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/server/analytics"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/milo/buildsource/buildbot/buildstore"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/frontend/ui"
	"go.chromium.org/luci/milo/git"
)

// A collection of useful templating functions

// funcMap is what gets fed into the template bundle.
var funcMap = template.FuncMap{
	"faviconMIMEType":  faviconMIMEType,
	"formatCommitDesc": formatCommitDesc,
	"formatTime":       formatTime,
	"humanDuration":    humanDuration,
	"localTime":        localTime,
	"localTimeTooltip": localTimeTooltip,
	"obfuscateEmail":   obfuscateEmail,
	"pagedURL":         pagedURL,
	"parseRFC3339":     parseRFC3339,
	"percent":          percent,
	"prefix":           prefix,
	"shortenEmail":     shortenEmail,
	"startswith":       strings.HasPrefix,
	"sub":              sub,
	"toLower":          strings.ToLower,
}

// localTime returns a <span> element with t in human format
// that will be converted to local timezone in the browser.
// Recommended usage: {{ .Date | localTime "N/A" }}
func localTime(ifZero string, t time.Time) template.HTML {
	return localTimeCommon(ifZero, t, "", t.Format(time.RFC850))
}

// localTimeTooltip is similar to localTime, but shows time in a tooltip and
// allows to specify inner text to be added to the created <span> element.
// Recommended usage: {{ .Date | localTimeTooltip "innerText" "N/A" }}
func localTimeTooltip(innerText string, ifZero string, t time.Time) template.HTML {
	return localTimeCommon(ifZero, t, "tooltip-only", innerText)
}

func localTimeCommon(ifZero string, t time.Time, tooltipClass string, innerText string) template.HTML {
	if t.IsZero() {
		return template.HTML(template.HTMLEscapeString(ifZero))
	}
	milliseconds := t.UnixNano() / 1e6
	return template.HTML(fmt.Sprintf(
		`<span class="local-time %s" data-timestamp="%d">%s</span>`,
		tooltipClass,
		milliseconds,
		template.HTMLEscapeString(innerText)))
}

// rURL matches anything that looks like an https:// URL.
var rURL = regexp.MustCompile(`\bhttps://\S*\b`)

// rBUGLINE matches a bug line in a commit, including if it is quoted.
// Expected formats: "BUG: 1234,1234", "bugs=1234", "  >  >  BUG: 123"
// We use &gt; for > because this needs to deal with HTML escaped text.
var rBUGLINE = regexp.MustCompile(`(?m)^(&gt;| )*(?i:bugs?)[:=].+$`)

// rBUG matches expected items in a bug line.  Expected format: 12345, project:12345, #12345
var rBUG = regexp.MustCompile(`\b(\w+:)?#?\d+\b`)

// Expected formats: b/123456, crbug/123456, crbug/project/123456, crbug:123456, etc.
var rBUGLINK = regexp.MustCompile(`\b(b|crbug(\.com)?([:/]\w+)?)[:/]\d+\b`)

// tURL is a URL template.
var tURL = template.Must(template.New("tURL").Parse("<a href=\"{{.URL}}\">{{.Label}}</a>"))

type link struct {
	Label string
	URL   string
}

func makeLink(label, href string) string {
	buf := bytes.Buffer{}
	if err := tURL.Execute(&buf, link{label, href}); err != nil {
		return label
	}
	return buf.String()
}

// formatCommitDesc takes a commit message and adds embellishments such as:
// * Linkify https:// URLs
// * Linkify bug numbers using https://crbug.com/
// * Linkify b/ bug links
// * Linkify crbug/ bug links
func formatCommitDesc(desc string) template.HTML {
	// Since we take in a string and return a trusted raw HTML string, escape
	// everything first.
	desc = template.HTMLEscapeString(desc)
	// Replace https:// URLs
	result := rURL.ReplaceAllStringFunc(desc, func(s string) string {
		return makeLink(s, s)
	})
	// Replace b/ and crbug/ URLs
	result = rBUGLINK.ReplaceAllStringFunc(result, func(s string) string {
		// Normalize separator.
		u := strings.Replace(s, ":", "/", -1)
		u = strings.Replace(u, "crbug/", "crbug.com/", 1)
		scheme := "https://"
		if strings.HasPrefix(u, "b/") {
			scheme = "http://"
		}
		return makeLink(s, scheme+u)
	})
	// Replace BUG: lines with URLs by rewriting all bug numbers with links.
	return template.HTML(rBUGLINE.ReplaceAllStringFunc(result, func(s string) string {
		return rBUG.ReplaceAllStringFunc(s, func(sBug string) string {
			path := strings.Replace(strings.Replace(sBug, "#", "", 1), ":", "/", 1)
			return makeLink(sBug, "https://crbug.com/"+path)
		})
	}))
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

// prefix abbriviates a string into specified number of characters.
// Recommended usage: {{ .GitHash | prefix 8 }}
func prefix(prefixLen int, s string) string {
	if len(s) > prefixLen {
		return s[:prefixLen]
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
		DefaultArgs: func(c context.Context, e *templates.Extra) (templates.Args, error) {
			loginURL, err := auth.LoginURL(c, e.Request.URL.RequestURI())
			if err != nil {
				return nil, err
			}
			logoutURL, err := auth.LogoutURL(c, e.Request.URL.RequestURI())
			if err != nil {
				return nil, err
			}

			project := e.Params.ByName("project")
			group := e.Params.ByName("group")

			return templates.Args{
				"AppVersion":  strings.Split(info.VersionID(c), ".")[0],
				"IsAnonymous": auth.CurrentIdentity(c) == identity.AnonymousIdentity,
				"User":        auth.CurrentUser(c),
				"LoginURL":    loginURL,
				"LogoutURL":   logoutURL,
				"CurrentTime": clock.Now(c),
				"Analytics":   analytics.Snippet(c),
				"RequestID":   info.RequestID(c),
				"Request":     e.Request,
				"Navi":        ProjectLinks(c, project, group),
				"ProjectID":   project,
			}, nil
		},
		FuncMap: funcMap,
	}
}

// withGitilesMiddleware is a middleware that installs a prod Gitiles client
// factory into the context.
func withGitilesMiddleware(c *router.Context, next router.Handler) {
	c.Context = git.UseFactory(c.Context, git.GitilesProdClient)
	next(c)
}

// withGerritMiddleware is a middleware that installs a prod Gerrit client
// factory into the context that creates a prod Gerrit client that uses
// Milo's credentials.
func withGerritMiddleware(c *router.Context, next router.Handler) {
	c.Context = common.WithGerritFactory(c.Context, func(c context.Context, host string) (gerritpb.GerritClient, error) {
		t, auth, err := git.Transport(c, host)
		if err != nil {
			return nil, err
		}
		return gerrit.NewRESTClient(&http.Client{Transport: t}, host, auth)
	})
	next(c)
}

// withAccessClientMiddleware is a middleware that installs a prod buildbucket
// access API client into the context.
//
// This middleware depends on auth middleware in order to generate the access
// client, so it must be called after the auth middleware is installed.
func withAccessClientMiddleware(c *router.Context, next router.Handler) {
	client, err := common.NewAccessClient(c.Context)
	if err != nil {
		ErrorHandler(c, err)
		return
	}
	c.Context = common.WithAccessClient(c.Context, client)
	next(c)
}

// projectACLMiddleware adds ACL checks on a per-project basis.
// Expects c.Params to have project parameter.
func projectACLMiddleware(c *router.Context, next router.Handler) {
	switch allowed, err := common.IsAllowed(c.Context, c.Params.ByName("project")); {
	case err != nil:
		ErrorHandler(c, err)
	case !allowed:
		if auth.CurrentIdentity(c.Context) == identity.AnonymousIdentity {
			ErrorHandler(c, errors.New("not logged in", common.CodeUnauthorized))
		} else {
			ErrorHandler(c, errors.New("no access to project", common.CodeNoAccess))
		}
	default:
		next(c)
	}
}

// emulationMiddleware enables buildstore emulation if "emulation" query
// string parameter is not empty.
func emulationMiddleware(c *router.Context, next router.Handler) {
	c.Context = buildstore.WithEmulation(c.Context, c.Request.FormValue("emulation") != "")
	next(c)
}

// ProjectLinks returns the navigation list surrounding a project and optionally group.
func ProjectLinks(c context.Context, project, group string) []ui.LinkGroup {
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
		groupLinks := []*ui.Link{}
		con, err := common.GetConsole(c, project, group)
		if err != nil {
			logging.WithError(err).Warningf(c, "error getting console")
		} else if !con.Def.BuilderViewOnly {
			groupLinks = append(groupLinks, ui.NewLink(
				"Console",
				fmt.Sprintf("/p/%s/g/%s/console", project, group),
				fmt.Sprintf("Console for group %s in project %s", group, project)))
		}

		groupLinks = append(groupLinks, ui.NewLink(
			"Builders",
			fmt.Sprintf("/p/%s/g/%s/builders", project, group),
			fmt.Sprintf("Builders for group %s in project %s", group, project)))

		links = append(links, ui.LinkGroup{
			Name:  ui.NewLink(group, "", ""),
			Links: groupLinks,
		})
	}
	return links
}
