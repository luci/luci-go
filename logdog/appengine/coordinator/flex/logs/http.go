// Copyright 2018 The LUCI Authors.
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

package logs

import (
	"context"
	"fmt"
	"html"
	"html/template"
	"image/color"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes"

	"google.golang.org/grpc/codes"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/appengine/coordinator/flex"
	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
)

const (
	// maxBuffer specifies that maximum number of log entries we can buffer at a time.
	// A single log entry is 1MB at most.
	maxBuffer = 20

	// maxBackoff specifies the maximum amount of time to wait between two storage requests.
	maxBackoff = time.Second * 30
)

var errRawRedirect = errors.Reason("HTML mode is not supported for binary or datagram").Err()

type header struct {
	Title       string
	Link        string
	IsAnonymous bool
	LoginURL    string
	LogoutURL   string
	UserPicture string
	UserEmail   string
	Path        string
	IsFull      bool // Full or Lite HTML mode.
}

// The stylesheet is inlined because static_dir isn't supported in flex.
// IMPORTANT: js/css assets are served via the "static" module/service.
// Note that in lite mode, <div id="lines"> has whitespace: pre; set,
// So whitespaces need to be added carefully.
var headerTemplate = template.Must(template.New("header").Parse(`
<!DOCTYPE html>
<head>
<html lang="en">
<meta charset="utf-8">
<meta name="google" value="notranslate">
<link rel="stylesheet" href="/static/css/viewer.css" type="text/css">
<link rel="stylesheet" href="/static/third_party/css/jquery-ui.min.css" type="text/css">
<script src="/static/third_party/js/moment-with-locales.min.js"></script>
<script src="/static/third_party/js/moment-timezone-with-data-2012-2022.min.js"></script>
<script src="/static/js/time.js"></script>
<script src="/static/third_party/js/jquery.min.js"></script>
<script src="/static/third_party/js/jquery-ui.min.js"></script>
<link id="favicon" rel="shortcut icon" type="image/png" href="https://storage.googleapis.com/chrome-infra/logdog-small.png">
<title>{{ .Title }} | LogDog </title></head>
<header>
<div>{{ if .Link }}<a href="{{ .Link }}">Back to build</a>{{ end }}</div>
<div>
<a id="to-raw" href="?format=raw">Raw log</a> |
{{ if .IsFull }}<a id="to-lite">Switch to lite
{{ else }}<a id="to-full">Switch to full{{ end }} mode</a> |
	{{ if .IsAnonymous }}
		<a href="{{.LoginURL}}" alt="Login">Login</a>
	{{ else }}
		{{ if .UserPicture }}
			<img class="account-picture" src="{{.UserPicture}}" alt="Account Icon">
		{{ end }}
		{{ .UserEmail }} |
		<a href="{{.LogoutURL}}" alt="Logout">Logout</a>
	{{ end }}
</div></header><hr>
<script src="/static/js/viewer.js"></script>
<div class="lines {{ if .IsFull }}full{{ else }}lite{{ end }}">`))

type footer struct {
	Message  string
	Duration string
}

var footerTemplate = template.Must(template.New("footer").Parse(`
</div>
<hr><footer>
<div><img class="logdog-logo" src="https://storage.googleapis.com/chrome-infra/logdog-small.png"></div>
<div>{{ .Message }}<br>
This is <a href="https://chromium.googlesource.com/infra/luci/luci-go/+/master/logdog/">LogDog</a><br>
Rendering took {{ .Duration }}</div>
</footer>
`))

// logResp is the response object passed from the fetch routine back to the rendering routine.
// If both log and err are nil, that is a signal that the fetch routine is sleeping.
// That would be a good time to flush any output.
type logResp struct {
	stream *coordinator.LogStream
	log    *logpb.LogEntry
	err    error
}

// logData contains content and metadata for a log stream.
// The content is accessed via ch.
type logData struct {
	ch <-chan logResp
	// logStreams hold the final resolved list of logStreams we are fetching.
	// This is used by the renderer to pull out viewer_url tags and set content-type.
	logStreams []*coordinator.LogStream
	options    userOptions
}

// viewerURL is a convenience function to extract the logdog.viewer_url tag
// out of a logStream, if available.
// If given multiple streams, it will return the first one found with the viewer_url tag.
func (ld *logData) viewerURL() string {
	for _, s := range ld.logStreams {
		desc, err := s.DescriptorValue()
		if err != nil {
			continue
		}
		if u, ok := desc.Tags["logdog.viewer_url"]; ok {
			return u
		}
	}
	return ""
}

// nopFlusher is an http.Flusher that does nothing.  It's used as a standin
// if no http.Flusher is installed in the ResponseWriter.
type nopFlusher struct{}

func (n *nopFlusher) Flush() {} // Do nothing

const (
	formatRAW      = "raw"
	formatHTMLLite = "lite"
	formatHTMLFull = "full"
)

// userOptions encapsulate the entirety of input parameters to the log viewer.
type userOptions struct {
	// project is the name of the requested logdog project.
	project types.ProjectName
	// path is the full path (prefix + name) of the requested log stream.
	path types.StreamPath
	// wildcard indicates that the name contains wildcards ("*" or "**").
	wildcard bool
	// format indicates the format the user wants the data back in.
	// Valid formats are "raw", "lite", and "full.
	// If the user specifies "?format=html",
	// it will get resolved to either "lite" or "full" depending on:
	//   * What cookies are set.
	//   * Whether or not a URL fragment is in the path.
	format string
}

func (uo userOptions) isHTML() bool {
	return uo.format == formatHTMLLite || uo.format == formatHTMLFull
}

// resolveFormat resolves the output format to serve to the user.
// This could be "html", "lite", "full", or "raw".
// Here we try to be smart, and detect if the user is a web browser, or a CLI tool (E.G. cURL).
// For web browsers, default to HTML mode, unless ?format=raw is specified.
// For CLI tools, default to raw mode, unless ?format=html is specified.
// HTML mode has two modes, "lite" and "full".  Lite is default, unless:
//   * The user has a cookie specifying preference to full mode.
//   * A URL fragment is detected in the path.
// If we can't figure anything out, default to HTML lite mode.
// We do this before path parsing to figure out which mode we want
// to render parsing errors in.
func resolveFormat(request *http.Request) string {
	// If a known format is specified, return it.
	format := request.URL.Query().Get("format")
	switch f := strings.ToLower(format); f {
	case formatHTMLLite, formatHTMLFull, formatRAW:
		return f
	}
	// TODO(hinoka): Check accept header first.
	// User Agents are basically formatted as "<Type>/<Version> <other stuff>"
	// We really only care about the very first <Type> string.
	// from there we can basically differentiate between major classes of user agents. (Browser vs CLI)
	if format == "" {
		clientType := strings.SplitN(request.UserAgent(), "/", 2)[0]
		switch strings.ToLower(clientType) {
		case "curl", "wget", "python-urllib":
			return formatRAW
		}
	}

	// Default to some sort of HTML mode, decide if it's full or lite.
	if request.URL.Fragment != "" {
		return formatHTMLFull // URL fragment requires full mode.
	}
	if cookie, err := request.Cookie("html-mode"); err == nil && cookie.Value == formatHTMLFull {
		return formatHTMLFull
	}
	return formatHTMLLite
}

// resolveOptions takes the request path and http request, and returns the
// full set of requested options.
// pathStr is a path to one or more streams in a prefix.  Format includes:
// * Fully resolved path.  a/+/b/c
// * Single star path.  a/+/b/*
// * Double star path. a/+/**
func resolveOptions(request *http.Request, pathStr string) (options userOptions, err error) {
	options.format = resolveFormat(request)

	// The expected format is "/project/path..."
	parts := strings.SplitN(strings.TrimLeft(pathStr, "/"), "/", 2)
	if len(parts) != 2 {
		err = errors.Reason("invalid path %q", pathStr).Err()
		return
	}
	options.path = types.StreamPath(parts[1])
	prefix, name := options.path.Split()
	if err = prefix.Validate(); err != nil {
		return
	}
	if strings.Contains(name.String(), "*") {
		options.wildcard = true
	} else if err = name.Validate(); err != nil {
		// Validate name, if it is a fully resolved path.
		return
	}
	options.project = types.ProjectName(parts[0])
	err = options.project.Validate()
	return
}

// matchStreams returns streams that:
// * Are not purged.
// * Matches the path pattern.
func matchStreams(streams []*coordinator.LogStream, pattern string) []*coordinator.LogStream {
	// Filter results that match the requested stream name.
	// ** turns into .*
	// * turns into [^/]*
	doubles := strings.Split(pattern, "**")
	for i, d := range doubles {
		singles := strings.Split(d, "*")
		for j, s := range singles {
			singles[j] = regexp.QuoteMeta(s)
		}
		doubles[i] = strings.Join(singles, "[^/]*")
	}
	r := regexp.MustCompile(fmt.Sprintf("^%s$", strings.Join(doubles, ".*")))
	result := make([]*coordinator.LogStream, 0, len(streams))
	for _, stream := range streams {
		if stream.Purged {
			continue
		}
		if stream.StreamType != logpb.StreamType_TEXT {
			// Multistream viewing only supported for text streams.
			continue
		}
		if r.MatchString(stream.Name) {
			result = append(result, stream)
		}
	}
	return result
}

// resolveStreams takes a path containing a wildcard and resolves it to the list
// of all known paths.  Purge checks are also performed here.
func resolveStreams(c context.Context, options userOptions) ([]*coordinator.LogStream, error) {
	prefix, name := options.path.Split()
	streams := []*coordinator.LogStream{}
	var err error
	if options.wildcard {
		q := datastore.NewQuery("LogStream").Eq("Prefix", prefix.String()).Eq("Purged", false)
		err = datastore.GetAll(c, q, &streams)
	} else {
		streams = append(streams, &coordinator.LogStream{ID: coordinator.LogStreamID(options.path)})
		err = datastore.Get(c, streams[0])
	}

	switch {
	case len(streams) == 0, datastore.IsErrNoSuchEntity(err):
		return nil, coordinator.ErrPathNotFound
	case err != nil:
		return nil, err
	case !options.wildcard:
		// If the user requested HTML format, check to see if the stream is a binary or datagram stream.
		// If so, then this mode is not supported, and instead just render a link to the supported mode.
		if options.isHTML() && streams[0].StreamType != logpb.StreamType_TEXT {
			err = errRawRedirect
		}
		return streams, err
	}

	result := matchStreams(streams, name.String())
	if len(result) == 0 {
		return nil, coordinator.ErrPathNotFound
	}
	return result, nil
}

// initParams generates a set of params for each LogStream given.
func initParams(c context.Context, streams []*coordinator.LogStream, project types.ProjectName) ([]fetchParams, error) {
	states := make([]*coordinator.LogStreamState, len(streams))
	for i, stream := range streams {
		states[i] = stream.State(c)
	}
	// Get the log states, which we use to fetch the backend storage.
	switch err := datastore.Get(c, states); {
	case datastore.IsErrNoSuchEntity(err):
		return nil, coordinator.ErrPathNotFound
	case err != nil:
		return nil, err
	}

	// Get the backend storage instances.  These will be closed in fetch.
	params := make([]fetchParams, len(streams))
	for i, stream := range streams {
		st, err := flex.GetServices(c).StorageForStream(c, states[i], project)
		if err != nil {
			return nil, err
		}
		params[i] = fetchParams{
			storage: st,
			stream:  stream,
			state:   states[i],
		}
	}

	return params, nil
}

// startFetch sets up the backend storage instance (Either BigTable or Google Storage)
// and then jump starts the goroutine to fetch logs from the backend storage.
// It returns a logData struct containing:
// * A channel where logs entries are sent back.
// * Log Stream metadata and state.
func startFetch(c context.Context, request *http.Request, pathStr string) (data logData, err error) {
	if data.options, err = resolveOptions(request, pathStr); err != nil {
		err = errors.Annotate(err, "resolving options").Tag(grpcutil.InvalidArgumentTag).Err()
		return
	}
	// Pull out project/path for convenience.
	project, path := data.options.project, data.options.path
	logging.Fields{
		"project": project,
		"path":    path,
		"format":  data.options.format,
	}.Debugf(c, "parsed options")

	// Enter project namespace.  This is effectively the ACL check.
	// If successful, the context will be modified to enter the project datastore namespace.
	if err = coordinator.WithProjectNamespace(&c, project, coordinator.NamespaceAccessREAD); err != nil {
		return
	}

	// Enumerate all of the streams we will be fetching from.
	// If the input path is a fully qualified path, this will return exactly 1 stream.
	streams, err := resolveStreams(c, data.options)
	if err != nil {
		return
	}

	params, err := initParams(c, streams, project)
	if err != nil {
		return
	}

	// Create a channel to transfer log data.  This channel will be closed by
	// fetch() to signal that all logs have been returned (or an error was encountered).
	ch := make(chan logResp, maxBuffer)
	// Start fetching!!!
	go fetch(c, ch, params)
	data.ch = ch
	data.logStreams = streams
	return
}

// fetchParams are the set of parameters required for fetching logs at a path.
type fetchParams struct {
	storage storage.Storage
	stream  *coordinator.LogStream
	state   *coordinator.LogStreamState
}

// fetch is a goroutine that fetches log entries from all storage layers and
// sends it through ch in the order of prefix index.
// The LogStreamState is used purely for checking Terminal Indices.
func fetch(c context.Context, ch chan<- logResp, allParams []fetchParams) {
	defer close(ch) // Close the channel when we're done fetching.

	// TODO(hinoka): Remove this check once multistream view is supported.
	if len(allParams) > 1 {
		streamPaths := make([]string, len(allParams))
		for i, p := range allParams {
			streamPaths[i] = string(p.stream.Path())
			p.storage.Close()
		}
		ch <- logResp{nil, nil, fmt.Errorf("Only single stream supported, found %d: %s", len(allParams), strings.Join(streamPaths, "\n"))}
		return
	}

	// Extract the params we need.
	params := allParams[0]
	state := params.state
	st := params.storage
	defer st.Close() // Close the connection to the backend when we're done.

	index := types.MessageIndex(0)
	backoff := time.Second // How long to wait between fetch requests from storage.
	var err error
	for {
		// Get the terminal index from the LogStreamState again if we need to.
		// This is useful when the log stream terminated after the user request started.
		if !state.Terminated() {
			if err = datastore.Get(c, state); err != nil {
				ch <- logResp{nil, nil, err}
				return
			}
		}

		// Do the actual work.
		nextIndex, err := fetchOnce(c, ch, index, params)
		// Signal regardless of error.  Server will bail on error and flush otherwise.
		// Important: if fetchOnce errors out, we still expect nextIndex to increment regardless,
		// otherwise this may get stuck in an infinite loop.
		ch <- logResp{nil, nil, err}

		// Check if the log has finished streaming and if we're done.
		if state.Terminated() && nextIndex > types.MessageIndex(state.TerminalIndex) {
			logging.Fields{
				"terminalIndex": state.TerminalIndex,
				"currentIndex":  index,
			}.Debugf(c, "Finished streaming")
			return
		}

		if err != nil {
			index = nextIndex
			continue
		}

		// Log is still streaming.  Set the next index, sleep a bit and try again.
		backoff = backoff * 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
		if index != nextIndex {
			if err != nil {
				// An error on one log entry means we want to try the next one asap.
				backoff = 0
			} else {
				// If its a relatively active log stream, don't sleep too long.
				backoff = time.Second
			}
		}
		if tr := clock.Sleep(c, backoff); tr.Err != nil {
			// If the user cancelled the request, bail out instead.
			ch <- logResp{nil, nil, tr.Err}
			return
		}
		index = nextIndex
	}
}

// fetchOnce does one backend storage request for logs, which may produce multiple log entries.
//
// Log entries are pushed into ch.
// Returns the next stream index to fetch.
// It is possible for the next index to be the terminal index + 1, so the calling
// code has to test for that.
func fetchOnce(c context.Context, ch chan<- logResp, index types.MessageIndex, params fetchParams) (types.MessageIndex, error) {
	// Extract just the parameters we need.
	st := params.storage // This is the backend storage instance.
	archived := params.state.ArchivalState().Archived()
	path := params.stream.Path()

	req := storage.GetRequest{
		Project: coordinator.Project(c),
		Path:    path,
		Index:   index,
	}
	var ierr error
	// Get as many log entries as we can.  The storage layer is responsible for low level retries.
	err := st.Get(c, req, func(e *storage.Entry) bool {
		var le *logpb.LogEntry
		if le, ierr = e.GetLogEntry(); ierr != nil {
			// This log entry is bad, just try the next one blindly.
			index++
			return false
		}
		sidx, _ := e.GetStreamIndex() // GetLogEntry succeeded, so this must.
		// Check for a non-continguous stream index.  This could mean a couple things:
		// Still streaming: The backend probably needs to catch up.  Return false and retry the fetch.
		// Archived: We permanently lost the log entries, just skip over them.
		if sidx != index && !archived {
			if archived {
				logging.Fields{"lostLogs": true}.Errorf(c, "Oh no we lost some logs.")
			} else {
				logging.Debugf(c, "Got a non-contiguous stream index, backend needs to catch up.")
			}
			return false
		}
		// Send the log entry over!  This may block if the channel buffer is full.
		ch <- logResp{params.stream, le, nil}
		index = sidx + 1
		return true
	})
	if err != nil {
		index++
		logging.WithError(err).Debugf(c, "got some error while fetching, incrementing index to %d", index)
	}
	// TODO(hinoka): Handle not found case.
	if err == nil && ierr != nil {
		err = ierr
	}
	return index, err
}

func writeHTMLHeader(ctx *router.Context, data logData) {
	loginURL, err := auth.LoginURL(ctx.Context, ctx.Request.URL.RequestURI())
	if err != nil {
		logging.WithError(err).Errorf(ctx.Context, "getting login url")
		loginURL = "#ERROR"
	}
	logoutURL, err := auth.LogoutURL(ctx.Context, ctx.Request.URL.RequestURI())
	if err != nil {
		logging.WithError(err).Errorf(ctx.Context, "getting logout url")
		logoutURL = "#ERROR"
	}
	user := auth.CurrentUser(ctx.Context) // This will not return nil, so it's safe to access.
	project := data.options.project.String()
	path := data.options.path
	title := "error" // If the data is not filled in, this was probably an error.
	if project != "" && path != "" {
		title = fmt.Sprintf("%s | %s", project, path)
	}
	if err := headerTemplate.ExecuteTemplate(ctx.Writer, "header", header{
		Path:        fmt.Sprintf("%s/%s", data.options.project, data.options.path),
		Title:       title,
		Link:        data.viewerURL(),
		IsAnonymous: auth.CurrentIdentity(ctx.Context) == identity.AnonymousIdentity,
		LoginURL:    loginURL,
		LogoutURL:   logoutURL,
		UserPicture: user.Picture,
		UserEmail:   user.Email,
		IsFull:      data.options.format == formatHTMLFull,
	}); err != nil {
		fmt.Fprintf(ctx.Writer, "Failed to render page: %s", err)
	}
}

// writeOKHeaders writes the http response headers in accordence with the
// log stream type and user options.  The error is passed through.
func writeOKHeaders(ctx *router.Context, data logData) {
	// Tell nginx not to buffer anything.
	ctx.Writer.Header().Set("X-Accel-Buffering", "no")
	// Tell the browser to prefer https.
	ctx.Writer.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	// Set the correct content type based off the log stream and format.
	contentType := data.logStreams[0].ContentType
	if data.options.isHTML() {
		// Only text/html is supported for HTML mode.
		contentType = "text/html"
	}
	ctx.Writer.Header().Set("Content-Type", contentType)
	ctx.Writer.WriteHeader(http.StatusOK)
	if data.options.isHTML() {
		writeHTMLHeader(ctx, data)
	}
}

// writeErrorPage renders an error page.
func writeErrorPage(ctx *router.Context, err error, data logData) {
	ierr := errors.New("LogDog encountered an internal error")
	switch code := grpcutil.Code(err); code {
	case codes.Unauthenticated:
		// Redirect to login screen.
		var u string
		u, ierr = auth.LoginURL(ctx.Context, ctx.Request.URL.RequestURI())
		if ierr == nil {
			http.Redirect(ctx.Writer, ctx.Request, u, http.StatusFound)
			return
		}
		logging.WithError(ierr).Errorf(ctx.Context, "Error getting Login URL")
		fallthrough
	case codes.Internal:
		// Hide internal errors, expose all other errors.
		ctx.Writer.WriteHeader(http.StatusInternalServerError)
	default:
		ierr = err
		ctx.Writer.WriteHeader(grpcutil.CodeStatus(code))
	}
	if data.options.isHTML() {
		writeHTMLHeader(ctx, data)
	}
	writeFooter(ctx, clock.Now(ctx.Context), ierr, data.options.isHTML())
}

func writeFooter(ctx *router.Context, start time.Time, err error, isHTML bool) {
	var message string
	switch {
	case err == nil:
		// Pass
	case grpcutil.Code(err) == codes.Internal:
		message = "LogDog encountered error while serving logs."
	default:
		message = fmt.Sprintf("ERROR: %s", err)
	}

	if isHTML {
		if err := footerTemplate.ExecuteTemplate(ctx.Writer, "footer", footer{
			Message:  message,
			Duration: fmt.Sprintf("%.2fs", clock.Now(ctx.Context).Sub(start).Seconds()),
		}); err != nil {
			fmt.Fprintf(ctx.Writer, "Failed to render page: %s", html.EscapeString(err.Error()))
		}
	} else {
		fmt.Fprintf(ctx.Writer, message)
	}
	// TODO(hinoka): If there is an error, we can signal an error
	// by acquiring the underlying TCP connection using ctx.Writer.(http.Hijacker),
	// and then sending a TCP RST packet, which would manifest on
	// the client side as "connection reset by peer".
	// We'd want this so that if a user tries to curl a long log here, and it
	// fails, curl would also return a failure.
	// This way we don't silently serve someone bad data.
}

type durationInfoStruct struct {
	Delta     int64 // Delta since last output, in Milliseconds.
	Text      string
	TextColor string
	BGColor   string
}

func (d durationInfoStruct) Style() template.CSS {
	result := ""
	if d.TextColor != "" {
		result += fmt.Sprintf("color: %s; ", d.TextColor)
	}
	if d.BGColor != "" {
		result += fmt.Sprintf("background-color: %s; ", d.BGColor)
	}
	return template.CSS(result)
}

var white = color.RGBA{255, 255, 255, 0}
var liteyellow = color.RGBA{255, 254, 214, 0}
var yellow = color.RGBA{232, 220, 118, 0}
var red = color.RGBA{219, 37, 37, 0}

func linearScale(from, to uint8, scale float64) uint8 {
	fromF := float64(from)
	toF := float64(to)
	return uint8(fromF + (toF-fromF)*scale)
}

// gradient produces a color between from and to colors
// given a scale between 0.0 to 1.0, with each of the color
// components multiplied.
func gradient(from, to color.RGBA, scale float64) (result color.RGBA) {
	scale = math.Max(math.Min(scale, 1.0), 0.0)
	result.R = linearScale(from.R, to.R, scale)
	result.G = linearScale(from.G, to.G, scale)
	result.B = linearScale(from.B, to.B, scale)
	return
}

func hex(c color.RGBA) string {
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}

func durationInfo(previous, current time.Duration) (di durationInfoStruct) {
	s := current.Seconds()
	m := int(current.Minutes())
	switch {
	case current < 5*time.Minute:
		di.Text = fmt.Sprintf("%.1fs", s)
	case current < 60*time.Minute:
		di.Text = fmt.Sprintf("%dm %ds", m, int(s)%60)
	default:
		di.Text = fmt.Sprintf("%dh %dm", m/60, m%60)
	}
	if previous == 0 {
		return
	}
	delta := current - previous
	di.Delta = int64(delta / 1e6)
	// 0s - 1s:   No color change
	// 1s - 10s:  Neutral/White (#ffffff) to Yellow (#e8dc76)
	// 10s - 60s: Yellow (#e8dc76) to Red (#db2525)
	switch {
	case delta > 10*time.Second:
		// 10s = 0.0
		// 60s = 1.0
		scale := float64(delta-10*time.Second) / float64((60/10)*time.Second)
		di.BGColor = hex(gradient(yellow, red, scale))
		if scale > 0.5 {
			di.TextColor = hex(white)
		}
	case delta > time.Second:
		//  0s = 0.0
		// 10s = 1.0
		scale := float64(delta) / float64(time.Second*10)
		di.BGColor = hex(gradient(white, yellow, scale))
	}
	return
}

// logLineStruct is used by lineTemplate for rendering lines and line timestamps.
type logLineStruct struct {
	Text          string
	ID            string
	DataTimestamp int64 // Milliseconds since epoch.
	DurationInfo  durationInfoStruct
}

// errorTemplate is the HTML template used for lines that resulted in errors.
var errorTemplate = template.Must(template.New("error").Parse(`
<div class="error line">LOGDOG ERROR: {{.}}</div>`))

var lineTemplate = template.Must(template.New("line").Parse(`
<div class="line" id="{{.ID}}">
	<div class="timestamp" onclick="window.location.hash='{{.ID}}'"
			 data-timestamp="{{.DataTimestamp}}" data-delta="{{.DurationInfo.Delta}}"
			 onmouseover="utils.maybeFormatTime(this)" style="{{.DurationInfo.Style}}">
		{{.DurationInfo.Text}}
	</div>
	<span class="text">{{.Text}}</span>
</div>`))

// serve reads log entries from data.ch and writes into w.
func serve(c context.Context, data logData, w http.ResponseWriter) (err error) {
	// Note: Always put errors in merr instead of returning err.
	// The following defer will drop whatever err is in the named return
	// and replace it with merr.
	// This is done so that all errors can be aggregated.
	merr := errors.MultiError{}
	defer func() {
		if merr.First() == nil {
			err = nil
		} else {
			err = merr
		}
	}()

	flusher, ok := w.(http.Flusher)
	if !ok {
		logging.Errorf(c,
			"Could not obtain the flusher from the http.ResponseWriter. "+
				"The ResponseWriter was probably overridden by a middleware. "+
				"Logs will not stream correctly until this is fixed.")
		flusher = &nopFlusher{}
	}
	prevDuration := time.Duration(0)
	// Serve the logs.
	for logResp := range data.ch {
		log, ierr := logResp.log, logResp.err
		if ierr != nil {
			merr = append(merr, ierr)
			if ierr = errorTemplate.Execute(w, ierr); ierr != nil {
				merr = append(merr, ierr)
			}
			continue
		}

		for i, line := range log.GetText().GetLines() {
			// For html full mode, we escape and wrap each line with a div.
			// For html lite mode, just escape the line.
			// For raw mode, we just regurgitate the line.
			ierr = nil
			switch data.options.format {
			case formatHTMLFull:
				lt := logLineStruct{
					// Note: We want to use PrefixIndex because we might be viewing more than 1 stream.
					ID: fmt.Sprintf("L%d_%d", log.PrefixIndex, i),
					// The templating engine below escapes the lines for us.
					Text: string(line.GetValue()),
				}
				// Add in timestamp information, if available.
				duration, perr := ptypes.Duration(log.GetTimeOffset())
				if perr != nil {
					logging.WithError(perr).Debugf(c, "Got error while converting duration")
					duration = prevDuration
				}
				lt.DataTimestamp = logResp.stream.Timestamp.Add(duration).UnixNano() / 1e6
				lt.DurationInfo = durationInfo(prevDuration, duration)
				prevDuration = duration
				ierr = lineTemplate.Execute(w, lt)
			case formatHTMLLite:
				text := template.HTMLEscapeString(string(line.GetValue()))
				_, ierr = fmt.Fprintf(w, "%s\n", text)
			case formatRAW:
				_, ierr = fmt.Fprintf(w, "%s%s", line.GetValue(), line.GetDelimiter())
			default:
				panic("Impossible")
			}
			if ierr != nil {
				merr = append(merr, ierr)
			}
		}

		if log == nil {
			// Nil log is a signal that the fetcher completed a cycle of fetches
			// and is sleeping.  Flush out all the data in the ResponseWriter so that
			// the user can see it.
			// The go ResponseWriter will not flush until it reaches its threshold.
			flusher.Flush()
		}
	}
	return
}

// GetHandler is an HTTP handler for retrieving logs.
func GetHandler(ctx *router.Context) {
	start := clock.Now(ctx.Context)
	// Start the fetcher and wait for fetched logs to arrive into ch.
	data, err := startFetch(ctx.Context, ctx.Request, ctx.Params.ByName("path"))
	if err != nil {
		logging.WithError(err).Errorf(ctx.Context, "failed to start fetch")
		writeErrorPage(ctx, err, data)
		return
	}
	writeOKHeaders(ctx, data)

	// Write the log contents and then the footer.
	err = serve(ctx.Context, data, ctx.Writer)
	if err != nil {
		logging.WithError(err).Errorf(ctx.Context, "failed to serve logs")
	}
	writeFooter(ctx, start, err, data.options.isHTML())
}
