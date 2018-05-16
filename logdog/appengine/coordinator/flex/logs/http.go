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
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc/codes"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/appengine/coordinator/flex"
	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/server/router"
)

const (
	// maxBuffer specifies that maximum number of log entries we can buffer at a time.
	// A single log entry is 1MB at most.
	maxBuffer = 20

	// maxBackoff specifies the maximum amount of time to wait between two storage requests.
	maxBackoff = time.Second * 30
)

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
}

// nopFlusher is an http.Flusher that does nothing.  It's used as a standin
// if no http.Flusher is installed in the ResponseWriter.
type nopFlusher struct{}

func (n *nopFlusher) Flush() {} // Do nothing

// parse parses a log stream path and returns the internal path structs, if valid.
func parse(pathStr string) (project types.ProjectName, path types.StreamPath, err error) {
	// The expected format is "/project/path..."
	parts := strings.SplitN(strings.TrimLeft(pathStr, "/"), "/", 2)
	if len(parts) != 2 {
		err = errors.Reason("invalid path %q", pathStr).Err()
		return
	}
	path = types.StreamPath(parts[1])
	if err = path.Validate(); err != nil {
		return
	}
	project = types.ProjectName(parts[0])
	err = project.Validate()
	return
}

// startFetch sets up the backend storage instance (Either BigTable or Google Storage)
// and then jump starts the goroutine to fetch logs from the backend storage.
// It returns a logData struct containing:
// * A channel where logs entries are sent back.
// * Log Stream metadata and state.
func startFetch(c context.Context, pathStr string) (*logData, error) {
	project, path, err := parse(pathStr)
	if err != nil {
		return nil, err // TODO(hinoka): Tag this with 400
	}
	logging.Fields{
		"project": project,
		"path":    path,
	}.Debugf(c, "parsed path")

	// Perform the ACL checks, this also installs the project name into the namespace.
	// All datastore queries must use this context.
	var ls *coordinator.LogStream
	c, ls, err = coordinator.WithStream(c, project, path, coordinator.NamespaceAccessREAD)
	if err != nil {
		return nil, err
	}

	// Get the log state, which we use to fetch the backend storage.
	lst := ls.State(c)
	switch err := datastore.Get(c, lst); {
	case datastore.IsErrNoSuchEntity(err):
		return nil, coordinator.ErrPathNotFound
	case err != nil:
		return nil, err
	}

	// Get the backend storage instance.  This will be closed in fetch.
	st, err := flex.GetServices(c).StorageForStream(c, lst)
	if err != nil {
		return nil, err
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
	return &logData{ch, *ls, *lst}, nil
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

// writeOKHeaders writes the http response headers in accordence with the
// log stream type and user options.  The error is passed through.
func writeOKHeaders(w http.ResponseWriter, ls coordinator.LogStream) {
	// Tell nginx not to buffer anything.
	w.Header().Set("X-Accel-Buffering", "no")
	// Tell the browser to prefer https.
	w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	w.Header().Set("Content-Type", ls.ContentType)
	w.WriteHeader(http.StatusOK)
}

// writeErrHeaders writes the correct headers based on the error type.
func writeErrHeaders(w http.ResponseWriter, err error) {
	code := grpcutil.Code(err)
	if code == codes.Internal {
		// Hide internal errors, expose all other errors.
		http.Error(w, "LogDog encountered an internal error", http.StatusInternalServerError)
		return
	}
	// All errors tagged with a grpc code is meant for user consumption, so we
	// print it out to the user here.
	http.Error(w, "Error: "+err.Error(), grpcutil.CodeStatus(code))
}

// serve reads log entries from ch and writes into w.
func serve(c context.Context, ch <-chan logResp, w http.ResponseWriter) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		logging.Errorf(c,
			"Could not obtain the flusher from the http.ResponseWriter. "+
				"The ResponseWriter was probably overriden by a middleware. "+
				"Logs will not stream correctly until this is fixed.")
		flusher = &nopFlusher{}
	}
	// Serve the logs.
	for logResp := range ch {
		log, err := logResp.log, logResp.err
		if err != nil {
			return err
		}
		for _, line := range log.GetText().GetLines() {
			if _, err := fmt.Fprintln(w, line.GetValue()); err != nil {
				return errors.Reason("could not write line %q", line.GetValue()).Err()
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
	// Start the fetcher and wait for fetched logs to arrive into ch.
	data, err := startFetch(ctx.Context, ctx.Params.ByName("path"))
	if err != nil {
		logging.WithError(err).Errorf(ctx.Context, "failed to start fetch")
		writeErrHeaders(ctx.Writer, err)
		return
	}
	writeOKHeaders(ctx.Writer, data.logStream)

	// Write the log contents.
	if err := serve(ctx.Context, data.ch, ctx.Writer); err != nil {
		http.Error(ctx.Writer, "LogDog encountered error while serving logs.", http.StatusInternalServerError)
		logging.WithError(err).Errorf(ctx.Context, "failed to serve logs")
		// TODO(hinoka): We can signal an error by acquiring the underlying TCP connection
		// using ctx.Writer.(http.Hijacker), and then sending a TCP RST packet, which
		// would manifest on the client side as "connection reset by peer".
		// We'd want this so that if a user tries to curl a long log here, and it
		// fails, curl would also return a failure.  This way we don't silently
		// serve someone bad data.
	}
}
