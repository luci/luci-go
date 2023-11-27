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
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang/protobuf/ptypes"
	"github.com/russross/blackfriday/v2"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/text/sanitizehtml"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/gtm"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/milo/frontend/ui"
	"go.chromium.org/luci/milo/internal/buildsource/buildbucket"
	"go.chromium.org/luci/milo/internal/config"
	"go.chromium.org/luci/milo/internal/git"
	"go.chromium.org/luci/milo/internal/git/gitacls"
	"go.chromium.org/luci/milo/internal/projectconfig"
	"go.chromium.org/luci/milo/internal/utils"
)

// A collection of useful templating functions

// funcMap is what gets fed into the template bundle.
var funcMap = template.FuncMap{
	"botLink":          botLink,
	"logdogLink":       logdogLink,
	"duration":         utils.Duration,
	"faviconMIMEType":  faviconMIMEType,
	"formatCommitDesc": formatCommitDesc,
	"formatTime":       formatTime,
	"humanDuration":    utils.HumanDuration,
	"localTime":        localTime,
	"localTimestamp":   localTimestamp,
	"localTimeTooltip": localTimeTooltip,
	"obfuscateEmail":   utils.ObfuscateEmail,
	"pagedURL":         pagedURL,
	"parseRFC3339":     parseRFC3339,
	"percent":          percent,
	"prefix":           prefix,
	"renderMarkdown":   renderMarkdown,
	"sanitizeHTML":     sanitizeHTML,
	"shortenEmail":     utils.ShortenEmail,
	"startswith":       strings.HasPrefix,
	"sub":              sub,
	"toLower":          strings.ToLower,
	"toTime":           toTime,
	"join":             strings.Join,
	"trimLong":         trimLongString,
	"gitilesCommitURL": protoutil.GitilesCommitURL,
	"urlPathEscape":    url.PathEscape,
}

// trimLongString returns a potentially shortened string with "…" suffix.
// If maxRuneCount < 1, panics.
func trimLongString(maxRuneCount int, s string) string {
	if maxRuneCount < 1 {
		panic("maxRunCount must be >= 1")
	}

	if utf8.RuneCountInString(s) <= maxRuneCount {
		return s
	}

	// Take first maxRuneCount-1 runes.
	count := 0
	for i := range s {
		count++
		if count == maxRuneCount {
			return s[:i] + "…"
		}
	}
	panic("unreachable")
}

// localTime returns a <span> element with t in human format
// that will be converted to local timezone in the browser.
// Recommended usage: {{ .Date | localTime "N/A" }}
func localTime(ifZero string, t time.Time) template.HTML {
	return localTimeCommon(ifZero, t, "", t.Format(time.RFC850))
}

// localTimestamp is like localTime, but accepts a Timestamp protobuf.
func localTimestamp(ifZero string, ts *timestamppb.Timestamp) template.HTML {
	if ts == nil {
		return template.HTML(template.HTMLEscapeString(ifZero))
	}
	t := ts.AsTime()
	return localTime(ifZero, t)
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

// logdogLink generates a link with URL pointing to logdog host and a label
// corresponding to the log name. In case `raw` is true, the link points to the
// raw version of the log (without UI) and the label becomes "raw".
func logdogLink(log buildbucketpb.Log, raw bool) template.HTML {
	rawURL := "#invalid-logdog-link"
	if sa, err := types.ParseURL(log.Url); err == nil {
		u := url.URL{
			Scheme: "https",
			Host:   sa.Host,
			Path:   fmt.Sprintf("/logs/%s/%s", sa.Project, sa.Path),
		}
		if raw {
			u.RawQuery = "format=raw"
		}
		rawURL = u.String()
	}
	name := log.Name
	if raw {
		name = "raw"
	}
	return ui.NewLink(name, rawURL, fmt.Sprintf("raw log %s", log.Name)).HTML()
}

// rURL matches anything that looks like an https:// URL.
var rURL = regexp.MustCompile(`\bhttps://\S*\b`)

// rBUGLINE matches a bug line in a commit, including if it is quoted.
// Expected formats: "BUG: 1234,1234", "bugs=1234", "  >  >  BUG: 123"
var rBUGLINE = regexp.MustCompile(`(?m)^(>| )*(?i:bugs?)[:=].+$`)

// rBUG matches expected items in a bug line.  Expected format: 12345, project:12345, #12345
var rBUG = regexp.MustCompile(`\b(\w+:)?#?\d+\b`)

// Expected formats: b/123456, crbug/123456, crbug/project/123456, crbug:123456, etc.
var rBUGLINK = regexp.MustCompile(`\b(b|crbug(\.com)?([:/]\w+)?)[:/]\d+\b`)

// tURL is a URL template.
var tURL = template.Must(template.New("tURL").Parse("<a href=\"{{.URL}}\">{{.Label}}</a>"))

// formatChunk is either an already-processed trusted template.HTML, produced
// by some previous regexp, or an untrusted string that still needs escaping or
// further processing by regexps. We keep track of the distinction to avoid
// escaping twice and rewrite rules applying inside HTML tags.
//
// At most one of str or html will be non-empty. That one is the field in use.
type formatChunk struct {
	str  string
	html template.HTML
}

// replaceAllInChunks behaves like Regexp.ReplaceAllStringFunc, but it only
// acts on unprocessed elements of chunks. Already-processed elements are left
// as-is. repl returns trusted HTML, performing any necessary escaping.
func replaceAllInChunks(chunks []formatChunk, re *regexp.Regexp, repl func(string) template.HTML) []formatChunk {
	var ret []formatChunk
	for _, chunk := range chunks {
		if len(chunk.html) != 0 {
			ret = append(ret, chunk)
			continue
		}
		s := chunk.str
		for len(s) != 0 {
			loc := re.FindStringIndex(s)
			if loc == nil {
				ret = append(ret, formatChunk{str: s})
				break
			}
			if loc[0] > 0 {
				ret = append(ret, formatChunk{str: s[:loc[0]]})
			}
			html := repl(s[loc[0]:loc[1]])
			ret = append(ret, formatChunk{html: html})
			s = s[loc[1]:]
		}
	}
	return ret
}

// chunksToHTML concatenates chunks together, escaping as needed, to return a
// final completed HTML string.
func chunksToHTML(chunks []formatChunk) template.HTML {
	buf := bytes.Buffer{}
	for _, chunk := range chunks {
		if len(chunk.html) != 0 {
			buf.WriteString(string(chunk.html))
		} else {
			buf.WriteString(template.HTMLEscapeString(chunk.str))
		}
	}
	return template.HTML(buf.String())
}

type link struct {
	Label string
	URL   string
}

func makeLink(label, href string) template.HTML {
	buf := bytes.Buffer{}
	if err := tURL.Execute(&buf, link{label, href}); err != nil {
		return template.HTML(template.HTMLEscapeString(label))
	}
	return template.HTML(buf.String())
}

func replaceLinkChunks(chunks []formatChunk) []formatChunk {
	// Replace https:// URLs
	chunks = replaceAllInChunks(chunks, rURL, func(s string) template.HTML {
		return makeLink(s, s)
	})
	// Replace b/ and crbug/ URLs
	chunks = replaceAllInChunks(chunks, rBUGLINK, func(s string) template.HTML {
		// Normalize separator.
		u := strings.Replace(s, ":", "/", -1)
		u = strings.Replace(u, "crbug/", "crbug.com/", 1)
		scheme := "https://"
		if strings.HasPrefix(u, "b/") {
			scheme = "http://"
		}
		return makeLink(s, scheme+u)
	})
	return chunks
}

// botLink generates a link to a swarming bot given a buildbucketpb.BuildInfra_Swarming struct.
func botLink(s *buildbucketpb.BuildInfra_Swarming) (result template.HTML) {
	for _, d := range s.GetBotDimensions() {
		if d.Key == "id" {
			return ui.NewLink(
				d.Value,
				fmt.Sprintf("https://%s/bot?id=%s", s.Hostname, d.Value),
				fmt.Sprintf("swarming bot %s", d.Value)).HTML()
		}
	}
	return "N/A"
}

// formatCommitDesc takes a commit message and adds embellishments such as:
// * Linkify https:// URLs
// * Linkify bug numbers using https://crbug.com/
// * Linkify b/ bug links
// * Linkify crbug/ bug links
func formatCommitDesc(desc string) template.HTML {
	chunks := []formatChunk{{str: desc}}
	// Replace BUG: lines with URLs by rewriting all bug numbers with
	// links. Run this first so later rules do not interfere with it. This
	// allows a line like the following to work:
	//
	// Bug: https://crbug.com/1234, 5678
	chunks = replaceAllInChunks(chunks, rBUGLINE, func(s string) template.HTML {
		sChunks := []formatChunk{{str: s}}
		// The call later in the parent function will not reach into
		// sChunks, so run it separately.
		sChunks = replaceLinkChunks(sChunks)
		sChunks = replaceAllInChunks(sChunks, rBUG, func(sBug string) template.HTML {
			path := strings.Replace(strings.Replace(sBug, "#", "", 1), ":", "/", 1)
			return makeLink(sBug, "https://crbug.com/"+path)
		})
		return chunksToHTML(sChunks)
	})
	chunks = replaceLinkChunks(chunks)
	return chunksToHTML(chunks)
}

// toTime returns the time.Time format for the proto timestamp.
// If the proto timestamp is invalid, we return a zero-ed out time.Time.
func toTime(ts *timestamppb.Timestamp) (result time.Time) {
	// We want a zero-ed out time.Time, not one set to the epoch.
	if t, err := ptypes.Timestamp(ts); err == nil {
		result = t
	}
	return
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

// renderMarkdown renders the given text as markdown HTML.
// This uses blackfriday to convert from markdown to HTML,
// and sanitizehtml to allow only a small subset of HTML through.
func renderMarkdown(t string) (results template.HTML) {
	// We don't want auto punctuation, which changes "foo" into “foo”
	r := blackfriday.NewHTMLRenderer(blackfriday.HTMLRendererParameters{
		Flags: blackfriday.UseXHTML,
	})
	untrusted := blackfriday.Run(
		[]byte(t),
		blackfriday.WithRenderer(r),
		blackfriday.WithExtensions(blackfriday.NoIntraEmphasis|blackfriday.FencedCode|blackfriday.Autolink))
	out := bytes.NewBuffer(nil)
	if err := sanitizehtml.Sanitize(out, bytes.NewReader(untrusted)); err != nil {
		return template.HTML(fmt.Sprintf("Failed to render markdown: %s", template.HTMLEscapeString(err.Error())))
	}
	return template.HTML(out.String())
}

// sanitizeHTML sanitizes the given HTML.
// Only a limited set of tags is supported. See sanitizehtml.Sanitize for
// details.
func sanitizeHTML(s string) (results template.HTML) {
	out := bytes.NewBuffer(nil)
	// TODO(crbug/1119896): replace sanitizehtml with safehtml once its sanitizer
	// is exported.
	sanitizehtml.Sanitize(out, strings.NewReader(s))
	return template.HTML(out.String())
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
func getTemplateBundle(templatePath string, appVersionID string, prod bool) *templates.Bundle {
	return &templates.Bundle{
		Loader:          templates.FileSystemLoader(os.DirFS(templatePath)),
		DebugMode:       func(c context.Context) bool { return !prod },
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
			var reload *int
			if tReload := GetReload(e.Request, -1); tReload >= 0 {
				reload = &tReload
			}

			project := e.Params.ByName("project")
			group := e.Params.ByName("group")
			return templates.Args{
				"AppVersion":         appVersionID,
				"IsAnonymous":        auth.CurrentIdentity(c) == identity.AnonymousIdentity,
				"User":               auth.CurrentUser(c),
				"LoginURL":           loginURL,
				"LogoutURL":          logoutURL,
				"CurrentTime":        clock.Now(c),
				"GTMJSSnippet":       gtm.JSSnippet(c),
				"GTMNoScriptSnippet": gtm.NoScriptSnippet(c),
				"RequestID":          trace.SpanContextFromContext(c).TraceID().String(),
				"Request":            e.Request,
				"Navi":               ProjectLinks(c, project, group),
				"ProjectID":          project,
				"Reload":             reload,
			}, nil
		},
		FuncMap: funcMap,
	}
}

// withBuildbucketBuildsClient is a middleware that installs a production buildbucket builds RPC client into the context.
func withBuildbucketBuildsClient(c *router.Context, next router.Handler) {
	c.Request = c.Request.WithContext(buildbucket.WithBuildsClientFactory(c.Request.Context(), buildbucket.ProdBuildsClientFactory))
	next(c)
}

// withGitMiddleware is a middleware that installs a prod Gerrit and Gitiles client
// factory into the context. Both use Milo's credentials if current user is
// has been granted read access in settings.cfg.
//
// This middleware must be installed after the auth middleware.
func withGitMiddleware(c *router.Context, next router.Handler) {
	acls, err := gitacls.FromConfig(c.Request.Context(), config.GetSettings(c.Request.Context()).SourceAcls)
	if err != nil {
		ErrorHandler(c, err)
		return
	}
	c.Request = c.Request.WithContext(git.UseACLs(c.Request.Context(), acls))
	next(c)
}

// builds a projectACLMiddleware, which expects c.Params to have project
// parameter, adds ACL checks on a per-project basis, and install a git project
// into context.
// If optional is true, the returned middleware doesn't fail when the user has
// no access to the project.
func buildProjectACLMiddleware(optional bool) router.Middleware {
	return func(c *router.Context, next router.Handler) {
		luciProject := c.Params.ByName("project")
		switch allowed, err := projectconfig.IsAllowed(c.Request.Context(), luciProject); {
		case err != nil:
			ErrorHandler(c, err)
		case allowed:
			c.Request = c.Request.WithContext(git.WithProject(c.Request.Context(), luciProject))
			next(c)
		case !allowed && optional:
			next(c)
		default:
			if auth.CurrentIdentity(c.Request.Context()) == identity.AnonymousIdentity {
				ErrorHandler(c, errors.New("not logged in", grpcutil.UnauthenticatedTag))
			} else {
				ErrorHandler(c, errors.New("no access to project", grpcutil.PermissionDeniedTag))
			}
		}
	}
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
		con, err := projectconfig.GetConsole(c, project, group)
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
