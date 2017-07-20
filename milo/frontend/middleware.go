// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package frontend

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/gae/service/info"

	"github.com/luci/luci-go/appengine/gaeauth/server"
	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/server/analytics"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/identity"
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/templates"

	"github.com/luci/luci-go/milo/api/resp"
)

// A collection of useful templating functions

// funcMap is what gets fed into the template bundle.
var funcMap = template.FuncMap{
	"humanDuration":  humanDuration,
	"parseRFC3339":   parseRFC3339,
	"linkify":        linkify,
	"linkifySet":     linkifySet,
	"obfuscateEmail": obfuscateEmail,
	"localTime":      localTime,
	"shortHash":      shortHash,
	"startswith":     strings.HasPrefix,
	"sub":            sub,
	"pagedURL":       pagedURL,
	"formatTime":     formatTime,
	"percent":        percent,
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

// linkifyTemplate is the template used in "linkify". Because the template,
// itself recursively invokes "linkify", we will initialize it in explicitly
// in "init()".
//
// linkifySetTemplate is the template used in "linkifySet".
var linkifyTemplate, linkifySetTemplate *template.Template

// linkify turns a resp.LinkSet struct into a canonical link.
func linkify(link *resp.Link) template.HTML {
	if link == nil {
		return ""
	}
	buf := bytes.Buffer{}
	if err := linkifyTemplate.Execute(&buf, link); err != nil {
		panic(err)
	}
	return template.HTML(buf.Bytes())
}

// linkifySet turns a resp.Link struct into a canonical link.
func linkifySet(linkSet resp.LinkSet) template.HTML {
	if len(linkSet) == 0 {
		return ""
	}
	buf := bytes.Buffer{}
	if err := linkifySetTemplate.Execute(&buf, linkSet); err != nil {
		panic(err)
	}
	return template.HTML(buf.Bytes())
}

// sub subtracts one number from another, because apparently go templates aren't
// smart enough to do that.
func sub(a, b int) int {
	return a - b
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

func init() {
	linkifySetTemplate = template.Must(
		template.New("linkifySet").
			Funcs(template.FuncMap{
				"linkify": linkify,
			}).Parse(
			`{{ range $i, $link := . }}` +
				`{{ if gt $i 0 }} {{ end }}` +
				`{{ $link | linkify}}` +
				`{{ end }}`))

	linkifyTemplate = template.Must(
		template.New("linkify").
			Parse(
				`<a href="{{.URL}}">` +
					`{{if .Img}}<img src="{{.Img}}"{{if .Alt}} alt="{{.Alt}}"{{end}}>` +
					`{{else if .Alias}}[{{.Label}}]` +
					`{{else}}{{.Label}}{{end}}` +
					`</a>`))
}

var authconfig *auth.Config

// getTemplateBundles is used to render HTML templates. It provides base args
// passed to all templates.  It takes a path to the template folder, relative
// to the path of the binary during runtime.
func getTemplateBundle(templatePath string) *templates.Bundle {
	return &templates.Bundle{
		Loader:          templates.FileSystemLoader(templatePath),
		DebugMode:       info.IsDevAppServer,
		DefaultTemplate: "base",
		DefaultArgs: func(c context.Context) (templates.Args, error) {
			r := getRequest(c)
			path := r.URL.Path
			loginURL, err := auth.LoginURL(c, path)
			if err != nil {
				return nil, err
			}
			logoutURL, err := auth.LogoutURL(c, path)
			if err != nil {
				return nil, err
			}
			return templates.Args{
				"AppVersion":  strings.Split(info.VersionID(c), ".")[0],
				"IsAnonymous": auth.CurrentIdentity(c) == identity.AnonymousIdentity,
				"User":        auth.CurrentUser(c),
				"LoginURL":    loginURL,
				"LogoutURL":   logoutURL,
				"CurrentTime": clock.Now(c),
				"Analytics":   analytics.Snippet(c),
				"RequestID":   info.RequestID(c),
				"Request":     r,
			}, nil
		},
		FuncMap: funcMap,
	}
}

// base returns the basic LUCI appengine middlewares.
func base(templatePath string) router.MiddlewareChain {
	return gaemiddleware.BaseProd().Extend(
		auth.Authenticate(server.CookieAuth),
		withRequestMiddleware,
		templates.WithTemplates(getTemplateBundle(templatePath)),
	)
}

// The context key, so that we can embed the http.Request object into
// the context.
var requestKey = "http.request"

// withRequest returns a context with the http.Request object
// in it.
func withRequest(c context.Context, r *http.Request) context.Context {
	return context.WithValue(c, &requestKey, r)
}

// withRequestMiddleware is a middleware that installs a request into the context.
// This is used for various things in the default template.
func withRequestMiddleware(c *router.Context, next router.Handler) {
	c.Context = withRequest(c.Context, c.Request)
	next(c)
}

func getRequest(c context.Context) *http.Request {
	if req, ok := c.Value(&requestKey).(*http.Request); ok {
		return req
	}
	panic("No http.request found in context")
}
