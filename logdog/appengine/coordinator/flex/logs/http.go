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
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes"

	"google.golang.org/grpc/codes"

	"golang.org/x/net/context"

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
}

// The stylesheet is inlined because static_dir isn't supported in flex.
var headerTemplate = template.Must(template.New("header").Parse(`
<!DOCTYPE html>
<head>
<html lang="en">
<meta charset="utf-8">
<meta name="google" value="notranslate">
<style>
	body {
		font-family: Verdana, sans-serif;
		font-size: 14px;
		margin-left: 30px;
		margin-right: 30px;
		margin-top: 20px;
		margin-bottom: 50px;
	}
	header {
		display: flex;
		justify-content: space-between;
	}
	.account-picture {
		border-radius: 6px;
		width: 25px;
		height: 25px;
		vertical-align: middle;
	}
	.line {
		font-family: Courier New, Courier, monospace;
		font-size: 16px;
		white-space: pre;
	}
	footer {
		clear: both;
		font-size: 14px;
	}
	.logdog-logo {
		width: 4em;
		float: left;
		margin-right: 10px;
	}
</style>
<link id="favicon" rel="shortcut icon" type="image/png" href="https://storage.googleapis.com/chrome-infra/logdog-small.png">
<title>{{ .Title }} | LogDog </title></head>
<header>
<div>{{ if .Link }}<a href="{{ .Link }}">Back to build</a>{{ end }}</div>
<div>
<a href="/v/?s={{.Path}}">Use the old viewer</a> |
	{{ if .IsAnonymous }}
		<a href="{{.LoginURL}}" alt="Login">Login</a>
	{{ else }}
		{{ if .UserPicture }}
			<img class="account-picture" src="{{.UserPicture}}" alt="Account Icon">
		{{ end }}
		{{ .UserEmail }} |
		<a href="{{.LogoutURL}}" alt="Logout">Logout</a>
	{{ end }}
</div></header><hr><div class="lines">
<script>
  (function(i,s,o,g,r,a,m){i['CrDXObject']=r;i[r]=i[r]||function(){
  (i[r].q=i[r].q||[]).push(arguments)},a=s.createElement(o),
  m=s.getElementsByTagName(o)[0];a.async=1;a.src=g;m.parentNode.insertBefore(a,m)
  })(window,document,'script','https://storage.googleapis.com/crdx-feedback.appspot.com/feedback.js','crdx');

  crdx('setFeedbackButtonLink', 'https://bugs.chromium.org/p/chromium/issues/entry?components=Infra%3EPlatform%3ELogDog');
</script>
`))

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
	log *logpb.LogEntry
	err error
}

// logData contains content and metadata for a log stream.
// The content is accessed via ch.
type logData struct {
	ch             <-chan logResp
	logStream      coordinator.LogStream
	logStreamState coordinator.LogStreamState
	options        userOptions
}

// viewerURL is a convenience function to extract the logdog.viewer_url tag
// out of a logStream, if available.
func (ld *logData) viewerURL() string {
	desc, err := ld.logStream.DescriptorValue()
	if err != nil {
		return ""
	}
	return desc.Tags["logdog.viewer_url"]
}

// nopFlusher is an http.Flusher that does nothing.  It's used as a standin
// if no http.Flusher is installed in the ResponseWriter.
type nopFlusher struct{}

func (n *nopFlusher) Flush() {} // Do nothing

const (
	formatRAW  = "raw"
	formatHTML = "html"
)

// userOptions encapsulate the entirety of input parameters to the log viewer.
type userOptions struct {
	// project is the name of the requested logdog project.
	project types.ProjectName
	// path is the full path (prefix + name) of the requested log stream.
	path types.StreamPath
	// format indicates the format the user wants the data back in.
	// Valid formats are "raw" and "html".
	format string
}

// resolveFormat resolves the output format to serve to the user.  This could be "html" or "raw".
// Here we try to be smart, and detect if the user is a web browser, or a CLI tool (E.G. cURL).
// For web browsers, default to HTML mode, unless ?format=raw is specified.
// For CLI tools, default to raw mode, unless ?format=html is specified.
// If we can't figure anything out, default to HTML mode.
// We do this before path parsing to figure out which mode we want
// to render parsing errors in.
func resolveFormat(request *http.Request) string {
	// If a known format is specified, return it.
	format := request.URL.Query().Get("format")
	switch f := strings.ToLower(format); f {
	case formatHTML, formatRAW:
		return f
	}
	// User Agents are basically formatted as "<Type>/<Version> <other stuff>"
	// We really only care about the very first <Type> string.
	// from there we can basically differentiate between major classes of user agents. (Browser vs CLI)
	clientType := strings.SplitN(request.UserAgent(), "/", 2)[0]
	switch strings.ToLower(clientType) {
	case "curl", "wget", "python-urllib":
		return formatRAW
	default:
		return formatHTML
	}
}

// resolveOptions takes the request path and http request, and returns the
// full set of requested options.
func resolveOptions(request *http.Request, pathStr string) (options userOptions, err error) {
	options.format = resolveFormat(request)

	// The expected format is "/project/path..."
	parts := strings.SplitN(strings.TrimLeft(pathStr, "/"), "/", 2)
	if len(parts) != 2 {
		err = errors.Reason("invalid path %q", pathStr).Err()
		return
	}
	options.path = types.StreamPath(parts[1])
	if err = options.path.Validate(); err != nil {
		return
	}
	options.project = types.ProjectName(parts[0])
	err = options.project.Validate()
	return
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

	// Perform the ACL checks, this also installs the project name into the namespace.
	// All datastore queries must use this context.
	// Of note, if the request project is not found, this returns codes.PermissionDenied.
	// If the user is not logged in and the ACL checks fail, this returns codes.Unauthenticated.
	var ls *coordinator.LogStream
	c, ls, err = coordinator.WithStream(c, project, path, coordinator.NamespaceAccessREAD)
	if err != nil {
		return
	}

	// Get the log state, which we use to fetch the backend storage.
	lst := ls.State(c)
	switch err = datastore.Get(c, lst); {
	case datastore.IsErrNoSuchEntity(err):
		err = coordinator.ErrPathNotFound
		return
	case err != nil:
		return
	}

	// Get the backend storage instance.  This will be closed in fetch.
	st, err := flex.GetServices(c).StorageForStream(c, lst)
	if err != nil {
		return
	}

	// If the user requested HTML format, check to see if the stream is a binary or datagram stream.
	// If so, then this mode is not supported, and instead just render a link to the supported mode.
	if data.options.format == formatHTML {
		switch ls.StreamType {
		case logpb.StreamType_BINARY, logpb.StreamType_DATAGRAM:
			err = errRawRedirect
			return
		}
	}

	// Create a channel to transfer log data.  This channel will be closed by
	// fetch() to signal that all logs have been returned (or an error was encountered).
	ch := make(chan logResp, maxBuffer)
	params := fetchParams{
		path: path,
		st:   st,
		lst:  lst,
	}
	// Start fetching!
	go fetch(c, ch, params)
	// Return a snapshot of the log stream + state at this time,
	// since the fetcher is liable to modify it while fetching.
	data.ch = ch
	data.logStream = *ls
	data.logStreamState = *lst
	return
}

// fetchParams are the set of parameters required for fetching logs at a path.
type fetchParams struct {
	path types.StreamPath
	st   storage.Storage
	lst  *coordinator.LogStreamState
}

// fetch is a goroutine that fetches log entries from st at path and sends data back
// through ch.
// params.lst is used purely for checking Terminal Index.
func fetch(c context.Context, ch chan<- logResp, params fetchParams) {
	// Extract the params we need.
	lst := params.lst
	st := params.st

	defer close(ch)  // Close the channel when we're done fetching.
	defer st.Close() // Close the connection to the backend when we're done.
	index := types.MessageIndex(0)
	backoff := time.Second // How long to wait between fetch requests from storage.
	var err error
	for {
		// Get the terminal index from the LogStreamState again if we need to.
		// This is useful when the log stream terminated after the user request started.
		if !lst.Terminated() {
			if err = datastore.Get(c, lst); err != nil {
				ch <- logResp{nil, err}
				return
			}
		}

		// Do the actual work.
		nextIndex, err := fetchOnce(c, ch, index, params)
		// Signal regardless of error.  Server will bail on error and flush otherwise.
		ch <- logResp{nil, err}
		if err != nil {
			return
		}

		// Check if the log has finished streaming and if we're done.
		if lst.Terminated() && nextIndex > types.MessageIndex(lst.TerminalIndex) {
			logging.Fields{
				"terminalIndex": lst.TerminalIndex,
				"currentIndex":  index,
			}.Debugf(c, "Finished streaming")
			return
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
			ch <- logResp{nil, tr.Err}
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
	st := params.st // This is the backend storage instance.
	archived := params.lst.ArchivalState().Archived()
	path := params.path

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
		ch <- logResp{le, nil}
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
	contentType := data.logStream.ContentType
	if contentType == "text/plain" && data.options.format == formatHTML {
		contentType = "text/html"
	}
	ctx.Writer.Header().Set("Content-Type", contentType)
	ctx.Writer.WriteHeader(http.StatusOK)
	if data.options.format == formatHTML {
		writeHTMLHeader(ctx, data)
	}
}

// writeErrHeaders writes the correct headers based on the error type.
func writeErrHeaders(ctx *router.Context, err error, data logData) {
	message := "LogDog encountered an internal error"
	switch code := grpcutil.Code(err); code {
	case codes.Unauthenticated:
		// Redirect to login screen.
		u, err := auth.LoginURL(ctx.Context, ctx.Request.URL.RequestURI())
		if err == nil {
			http.Redirect(ctx.Writer, ctx.Request, u, http.StatusFound)
			return
		}
		logging.WithError(err).Errorf(ctx.Context, "Error getting Login URL")
		fallthrough
	case codes.Internal:
		// Hide internal errors, expose all other errors.
		ctx.Writer.WriteHeader(http.StatusInternalServerError)
	default:
		message = "Error: " + err.Error()
		ctx.Writer.WriteHeader(grpcutil.CodeStatus(code))
	}
	if data.options.format == formatHTML {
		writeHTMLHeader(ctx, data)
	}
	fmt.Fprintf(ctx.Writer, message)
}

func writeFooter(ctx *router.Context, start time.Time, err error, format string) {
	var message string
	switch {
	case err == nil:
		// Pass
	case grpcutil.Code(err) == codes.Internal:
		message = "LogDog encountered error while serving logs."
	default:
		message = fmt.Sprintf("ERROR: %s", err)
	}

	switch format {
	case formatHTML:
		if err := footerTemplate.ExecuteTemplate(ctx.Writer, "footer", footer{
			Message:  message,
			Duration: fmt.Sprintf("%.2fs", clock.Now(ctx.Context).Sub(start).Seconds()),
		}); err != nil {
			fmt.Fprintf(ctx.Writer, "Failed to render page: %s", err)
		}
	default:
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

// serve reads log entries from data.ch and writes into w.
func serve(c context.Context, data logData, w http.ResponseWriter) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		logging.Errorf(c,
			"Could not obtain the flusher from the http.ResponseWriter. "+
				"The ResponseWriter was probably overriden by a middleware. "+
				"Logs will not stream correctly until this is fixed.")
		flusher = &nopFlusher{}
	}
	// Serve the logs.
	for logResp := range data.ch {
		log, err := logResp.log, logResp.err
		if err != nil {
			return err
		}
		for _, line := range log.GetText().GetLines() {
			text := line.GetValue()
			if data.options.format == formatHTML {
				duration, err := ptypes.Duration(log.GetTimeOffset())
				if err != nil {
					text = fmt.Sprintf("<div class=\"line\">%s</div>", text)
				} else {
					t := data.logStream.Timestamp.Add(duration)
					text = fmt.Sprintf("<div class=\"line\" data-timestamp=\"%s\">%s</div>", t.Format(time.RFC3339), text)
				}
			}
			if _, err := fmt.Fprintln(w, text); err != nil {
				return errors.Reason("could not write line %q", text).Err()
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
	return nil
}

// GetHandler is an HTTP handler for retrieving logs.
func GetHandler(ctx *router.Context) {
	start := clock.Now(ctx.Context)
	// Start the fetcher and wait for fetched logs to arrive into ch.
	data, err := startFetch(ctx.Context, ctx.Request, ctx.Params.ByName("path"))
	if err != nil {
		logging.WithError(err).Errorf(ctx.Context, "failed to start fetch")
		writeErrHeaders(ctx, err, data)
		return
	}
	writeOKHeaders(ctx, data)

	// Write the log contents and then the footer.
	err = serve(ctx.Context, data, ctx.Writer)
	if err != nil {
		logging.WithError(err).Errorf(ctx.Context, "failed to serve logs")
	}
	writeFooter(ctx, start, err, data.options.format)
}
