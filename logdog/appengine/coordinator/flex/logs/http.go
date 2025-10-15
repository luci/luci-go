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
	"bytes"
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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/appengine/coordinator/flex"
	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/logdog/common/viewer"
)

const (
	// maxBuffer specifies that maximum number of log entries we can buffer at a time.
	// A single log entry is 1MB at most.
	maxBuffer = 20

	// maxBackoff specifies the maximum amount of time to wait between two storage requests.
	maxBackoff = time.Second * 30

	// maxErrors is the maximum errors we may encounter while fetching a log before stopping.
	maxErrors = 5
)

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
	desc *logpb.LogStreamDescriptor
	log  *logpb.LogEntry
	err  error
}

// logData contains content and metadata for a log stream.
// The content is accessed via ch.
type logData struct {
	ch <-chan logResp

	logDesc *logpb.LogStreamDescriptor // decoded logStream.Descriptor

	options userOptions
}

func (ld *logData) viewerURL() string {
	// logDesc can be nil if the log doesn't exist
	return ld.logDesc.GetTags()[viewer.LogDogViewerURLTag]
}

// nopFlusher is an http.Flusher that does nothing.  It's used as a standin
// if no http.Flusher is installed in the ResponseWriter.
type nopFlusher struct{}

func (n *nopFlusher) Flush() {} // Do nothing

const (
	formatRAW      = "raw"
	formatHTMLLite = "lite"
	formatHTMLFull = "full"
	formatLogcat   = "logcat"
)

// userOptions encapsulate the entirety of input parameters to the log viewer.
type userOptions struct {
	// project is the name of the requested logdog project.
	project string
	// path is the full path (prefix + name) of the requested log stream.
	path types.StreamPath
	// format indicates the format the user wants the data back in.
	// Valid formats are "raw", "lite", "logcat", and "full".
	// If the user specifies "?format=html",
	// it will get resolved to either "lite" or "full" depending on:
	//   * What cookies are set.
	//   * Whether or not a URL fragment is in the path.
	format string
	// packageName is the name of the package where only the logcat lines
	// belonging to that package will be displayed.
	// This is only applicable to the "logcat" format.
	packageName string
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
//   - The user has a cookie specifying preference to full mode.
//   - A URL fragment is detected in the path.
//
// If we can't figure anything out, default to HTML lite mode.
// We do this before path parsing to figure out which mode we want
// to render parsing errors in.
func resolveFormat(request *http.Request) string {
	// If a known format is specified, return it.
	format := request.URL.Query().Get("format")
	switch f := strings.ToLower(format); f {
	case formatHTMLLite, formatHTMLFull, formatRAW, formatLogcat:
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
		err = errors.Fmt("invalid path %q", pathStr)
		return
	}
	options.path = types.StreamPath(parts[1])
	if err = options.path.Validate(); err != nil {
		err = errors.Fmt("invalid stream path %q: %w", string(options.path), err)
		return
	}

	options.project = parts[0]
	err = config.ValidateProjectName(options.project)

	options.packageName = request.URL.Query().Get("package")
	return
}

// initParams generates a set of params for each LogStream given.
func initParams(c context.Context, stream *coordinator.LogStream, desc *logpb.LogStreamDescriptor, project string) (ret fetchParams, err error) {
	state := stream.State(c)
	switch err = datastore.Get(c, state); {
	case datastore.IsErrNoSuchEntity(err):
		err = coordinator.ErrPathNotFound
		return
	case err != nil:
		return
	}

	// Get the backend storage instances.  These will be closed in fetch.
	st, err := flex.GetServices(c).StorageForStream(c, state, project)
	if err != nil {
		return
	}
	return fetchParams{
		storage: st,
		stream:  stream,
		state:   state,
		desc:    desc,
	}, nil
}

// startFetch sets up the backend storage instance (Either BigTable or Google Storage)
// and then jump starts the goroutine to fetch logs from the backend storage.
// It returns a logData struct containing:
// * A channel where logs entries are sent back.
// * Log Stream metadata and state.
func startFetch(c context.Context, request *http.Request, pathStr string) (data logData, err error) {
	if data.options, err = resolveOptions(request, pathStr); err != nil {
		err = grpcutil.InvalidArgumentTag.Apply(errors.Fmt("resolving options: %w", err))
		return
	}
	// Pull out project/path for convenience.
	project, path := data.options.project, data.options.path
	logging.Fields{
		"project": project,
		"path":    path,
		"format":  data.options.format,
	}.Debugf(c, "parsed options")

	if err = coordinator.WithProjectNamespace(&c, project); err != nil {
		return
	}

	fetcher := coordinator.MetadataFetcher{Path: path}
	if err = fetcher.FetchWithACLCheck(c); err != nil {
		return
	}

	if data.logDesc, err = fetcher.Stream.DescriptorProto(); err != nil {
		return
	}

	param, err := initParams(c, fetcher.Stream, data.logDesc, project)
	if err != nil {
		return
	}

        // Don't fetch the stream for logcat mode (going to do a redirect).
        if data.options.format == formatLogcat {
                param.storage.Close()
                return
        }

	// Create a channel to transfer log data.  This channel will be closed by
	// fetch() to signal that all logs have been returned (or an error was encountered).
	ch := make(chan logResp, maxBuffer)
	// Start fetching!!!
	go fetch(c, ch, param)
	data.ch = ch
	return
}

// fetchParams are the set of parameters required for fetching logs at a path.
type fetchParams struct {
	storage storage.Storage
	stream  *coordinator.LogStream
	desc    *logpb.LogStreamDescriptor
	state   *coordinator.LogStreamState
}

// fetch is a goroutine that fetches log entries from all storage layers and
// sends it through ch in the order of prefix index.
// The LogStreamState is used purely for checking Terminal Indices.
func fetch(c context.Context, ch chan<- logResp, params fetchParams) {
	defer close(ch) // Close the channel when we're done fetching.

	state := params.state
	st := params.storage
	defer st.Close() // Close the connection to the backend when we're done.

	index := types.MessageIndex(0)
	backoff := time.Second // How long to wait between fetch requests from storage.
	errorsEncountered := 0
	var err error
	for {
		// Get the terminal index from the LogStreamState again if we need to.
		// This is useful when the log stream terminated after the user request started.
		if !state.Terminated() {
			if err = datastore.Get(c, state); err != nil {
				ch <- logResp{err: err}
				return
			}
		}

		// Do the actual work.
		nextIndex, err := fetchOnce(c, ch, index, params)
		// Signal regardless of error. Server will bail on error and flush otherwise.
		ch <- logResp{err: err}

		if err != nil {
			errorsEncountered++
			if errors.Is(err, storage.ErrDoesNotExist) || errorsEncountered >= maxErrors {
				// Error is fatal or we have run out of our allowed error quota.
				return
			}
			// We may be able to ride-through this error.
			// Try to get the next message.
			logging.WithError(err).Debugf(c, "got some error while fetching, incrementing index to %d", index)
			nextIndex = nextIndex + 1
		}

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
			// If its a relatively active log stream, don't sleep too long.
			backoff = time.Second
		}
		if tr := clock.Sleep(c, backoff); tr.Err != nil {
			// If the user cancelled the request, bail out instead.
			ch <- logResp{err: tr.Err}
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
		ch <- logResp{params.desc, le, nil}
		index = sidx + 1
		return true
	})
	// TODO(hinoka): Handle not found case.
	if err == nil && ierr != nil {
		err = ierr
	}
	return index, err
}

func writeHTMLHeader(ctx *router.Context, data logData) {
	loginURL, err := auth.LoginURL(ctx.Request.Context(), ctx.Request.URL.RequestURI())
	if err != nil {
		logging.WithError(err).Errorf(ctx.Request.Context(), "getting login url")
		loginURL = "#ERROR"
	}
	logoutURL, err := auth.LogoutURL(ctx.Request.Context(), ctx.Request.URL.RequestURI())
	if err != nil {
		logging.WithError(err).Errorf(ctx.Request.Context(), "getting logout url")
		logoutURL = "#ERROR"
	}
	user := auth.CurrentUser(ctx.Request.Context()) // This will not return nil, so it's safe to access.
	project := data.options.project
	path := data.options.path
	title := "error" // If the data is not filled in, this was probably an error.
	if project != "" && path != "" {
		title = fmt.Sprintf("%s | %s", project, path)
	}
	if err := headerTemplate.ExecuteTemplate(ctx.Writer, "header", header{
		Path:        fmt.Sprintf("%s/%s", data.options.project, data.options.path),
		Title:       title,
		Link:        data.viewerURL(),
		IsAnonymous: auth.CurrentIdentity(ctx.Request.Context()) == identity.AnonymousIdentity,
		LoginURL:    loginURL,
		LogoutURL:   logoutURL,
		UserPicture: user.Picture,
		UserEmail:   user.Email,
		IsFull:      data.options.format == formatHTMLFull,
	}); err != nil {
		fmt.Fprintf(ctx.Writer, "Failed to render page: %s", err)
	}
}

// writeOKHeaders writes the HTTP response headers in accordance with the log
// stream type and user options. The error is passed through.
func writeOKHeaders(ctx *router.Context, data logData) {
	// Tell nginx not to buffer anything.
	ctx.Writer.Header().Set("X-Accel-Buffering", "no")
	// Tell the browser to prefer HTTPS.
	ctx.Writer.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	// Set the correct content type based off the log stream and format.
	ctx.Writer.Header().Set("Content-Type", contentTypeHeader(data))
	ctx.Writer.WriteHeader(http.StatusOK)
	if data.options.isHTML() {
		writeHTMLHeader(ctx, data)
	}
}

// contentTypeHeader returns the HTTP Content-Type header value to use, given
// the content type from the log data.
func contentTypeHeader(data logData) string {
	contentType := data.logDesc.ContentType
	if data.options.isHTML() {
		// In the case of HTML, if no charset is specified we can assume it's
		// UTF-8, as that's the default.
		if contentType == "text/plain" {
			return "text/html; charset=utf-8"
		}
		// If the log is text, we can serve it as HTML and retain the charset.
		if strings.Contains(contentType, "text/plain; charset=") {
			contentType = strings.Replace(contentType, "plain", "html", 1)
		}
	}
	return contentType
}

// writeErrorPage renders an error page.
func writeErrorPage(ctx *router.Context, err error, data logData) {
	ierr := errors.New("LogDog encountered an internal error")
	switch code := grpcutil.Code(err); code {
	case codes.Unauthenticated:
		// Redirect to login screen.
		var u string
		u, ierr = auth.LoginURL(ctx.Request.Context(), ctx.Request.URL.RequestURI())
		if ierr == nil {
			http.Redirect(ctx.Writer, ctx.Request, u, http.StatusFound)
			return
		}
		logging.WithError(ierr).Errorf(ctx.Request.Context(), "Error getting Login URL")
		fallthrough
	case codes.Internal:
		logging.WithError(err).Errorf(ctx.Request.Context(), "handling logs request")

		// Hide internal errors, expose all other errors.
		ctx.Writer.WriteHeader(http.StatusInternalServerError)
	default:
		ierr = err
		code := grpcutil.CodeStatus(code)
		if code >= 500 {
			logging.WithError(err).Errorf(ctx.Request.Context(), "handling logs request")
		} else {
			logging.WithError(err).Warningf(ctx.Request.Context(), "handling logs request")
		}
		ctx.Writer.WriteHeader(code)
	}
	if data.options.isHTML() {
		writeHTMLHeader(ctx, data)
	}
	writeFooter(ctx, clock.Now(ctx.Request.Context()), ierr, data.options.isHTML())
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
			Duration: fmt.Sprintf("%.2fs", clock.Now(ctx.Request.Context()).Sub(start).Seconds()),
		}); err != nil {
			fmt.Fprintf(ctx.Writer, "Failed to render page: %s", html.EscapeString(err.Error()))
		}
	} else {
		fmt.Fprint(ctx.Writer, message)
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

var lineTemplate = template.Must(template.New("line").Funcs(template.FuncMap{
	"linkify": linkify,
}).Parse(`
<div class="line" id="{{.ID}}">
	<div class="timestamp" onclick="window.location.hash='{{.ID}}'"
			 data-timestamp="{{.DataTimestamp}}" data-delta="{{.DurationInfo.Delta}}"
			 onmouseover="utils.maybeFormatTime(this)" style="{{.DurationInfo.Style}}">
		{{.DurationInfo.Text}}
	</div>
	<span class="text">{{.Text | linkify}}</span>
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
			// For HTML full mode, we escape, linkify URLs in text,
			// and wrap each line with a div with timing information.
			// For HTML lite mode, just escape and linkify URLs.
			// For raw mode, we just regurgitate the line.
			ierr = nil
			switch data.options.format {
			case formatHTMLFull:
				lt := logLineStruct{
					// Note: We want to use PrefixIndex because we might be
					// viewing more than 1 stream.
					ID: fmt.Sprintf("L%d_%d", log.PrefixIndex, i),
					// The templating engine below escapes the lines for us.
					Text: string(line.GetValue()),
				}
				// Add in timestamp information, if available.
				var duration time.Duration
				if perr := log.GetTimeOffset().CheckValid(); perr != nil {
					logging.WithError(perr).Debugf(c, "Got error while converting duration")
					duration = prevDuration
				} else {
					duration = log.GetTimeOffset().AsDuration()
				}
				tStamp := logResp.desc.Timestamp.AsTime()
				lt.DataTimestamp = tStamp.Add(duration).UnixNano() / 1e6
				lt.DurationInfo = durationInfo(prevDuration, duration)
				prevDuration = duration
				ierr = lineTemplate.Execute(w, lt)
			case formatHTMLLite:
				text := linkify(string(line.GetValue()))
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
			// Nil log is a signal that the fetcher completed a cycle of
			// fetches and is sleeping. Flush out all the data in the
			// ResponseWriter so that the user can see it. The Go
			// ResponseWriter will not flush until it reaches its threshold.
			flusher.Flush()
		}
	}
	return
}

// GetHandler is an HTTP handler for retrieving logs.
func GetHandler(ctx *router.Context) {
	start := clock.Now(ctx.Request.Context())
	// Start the fetcher and wait for fetched logs to arrive into ch.
	path :=  ctx.Params.ByName("path")
	data, err := startFetch(ctx.Request.Context(), ctx.Request, path)
	if err != nil {
		err = errors.Fmt("start fetch: %w", err)
		writeErrorPage(ctx, err, data)
		return
	}

	// The logcat webapp is hosted under /static for simplicity. Redirect
	// there once we have checked ACLs on the logs.
	if data.options.format == formatLogcat {
		redirected_url := "/static/logcat.html#url=" + path
		if data.options.packageName != "" {
			redirected_url += "&package=" + data.options.packageName
		}
		http.Redirect(ctx.Writer, ctx.Request, redirected_url, http.StatusFound)
		return
	}
	writeOKHeaders(ctx, data)

	// Write the log contents and then the footer.
	err = serve(ctx.Request.Context(), data, ctx.Writer)
	if err != nil {
		logging.WithError(err).Errorf(ctx.Request.Context(), "failed to serve logs")
	}
	writeFooter(ctx, start, err, data.options.isHTML())
}

var linkTemplate = template.Must(template.New("link").Parse(`<a href="{{.}}">{{.}}</a>`))

var urlPattern = regexp.MustCompile(
	`\b(https?:\/\/` + // Only match URLs with http(s).
		`[\w-]+(\.[\w-]+)+` + // Domain name, may have word chars and dashes.
		`\.?(:\d+)?` + // Optional period, optional port.
		`(\/\S*)?)\b`) // Path, may be empty, may not have spaces.

// linkify converts URLs in log text to HTML links.
//
// It returns template.HTML, which indicates that the output is
// safe (escaped) HTML.
func linkify(line string) template.HTML {
	if !strings.Contains(line, "http") {
		// Initial quick check to avoid regexp check if there
		// are no URLs in the line. This is faster than the
		// regexp check below according to benchmarking.
		return template.HTML(template.HTMLEscapeString(line))
	}
	matchLocs := urlPattern.FindAllStringIndex(line, -1)
	if matchLocs == nil {
		return template.HTML(template.HTMLEscapeString(line))
	}
	b := new(bytes.Buffer)
	i := 0
	for _, loc := range matchLocs {
		s, e := loc[0], loc[1]
		// Escape and add the part before the link.
		if i < s {
			b.WriteString(template.HTMLEscapeString(line[i:s]))
		}
		// Write the link (and if that fails just escape).
		if err := linkTemplate.Execute(b, line[s:e]); err != nil {
			b.WriteString(template.HTMLEscapeString(line[s:e]))
		}
		i = e
	}
	if i < len(line) {
		// Escape and add the part after the link.
		b.WriteString(template.HTMLEscapeString(line[i:]))
	}
	return template.HTML(b.String())
}
