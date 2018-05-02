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

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/appengine/coordinator/flex"
	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/server/router"
)

// maxBuffer specifies that maximum number of log entries we can buffer at a time.
// A single log entry is 1MB at most.
var maxBuffer = 20

// logResp is the response object passed from the fetch routine back to the rendering routine.
type logResp struct {
	log *logpb.LogEntry
	err error
}

// logDesc contains information about the type of log stream.  This is used
// for serving the correct http and html headers.
type logDesc struct {
	logStream      *coordinator.LogStream
	logStreamState *coordinator.LogStreamState
}

// parse parses a log stream path and returns the internal path structs, if valid.
func parse(pathStr string) (project types.ProjectName, path types.StreamPath, err error) {
	// The expected format is "/project/path..."
	parts := strings.SplitN(pathStr, "/", 3)
	if len(parts) != 3 {
		err = fmt.Errorf("Invalid path %s", pathStr)
		return
	}
	path = types.StreamPath(parts[2])
	if err = path.Validate(); err != nil {
		return
	}
	project = types.ProjectName(parts[1])
	err = project.Validate()
	return
}

// startFetch sets up the backend storage instance (Either BigTable or Google Storage)
// and then jump starts the goroutine to fetch logs from the backend storage.
// It returns a channel where log entires are sent back.
func startFetch(c context.Context, pathStr string) (<-chan logResp, *logDesc, error) {
	project, path, err := parse(pathStr)
	if err != nil {
		return nil, nil, err // TODO(hinoka): Tag this with 400
	}
	logging.Fields{
		"project": project,
		"path":    path,
	}.Debugf(c, "parsed path")

	// Perform the ACL checks, this also installs the project name into the namespace.
	// All datastore queries must use this context.
	c, err = coordinator.WithAccess(c, project, path, coordinator.NamespaceAccessREAD)
	if err != nil {
		return nil, nil, err
	}

	// Get the log stream, which we use to fill out the log description.
	ls := &coordinator.LogStream{ID: coordinator.LogStreamID(path)}
	// And the log state, which we use to fetch the backend storage.
	lst := ls.State(c)
	if err := datastore.Get(c, ls, lst); err != nil {
		return nil, nil, err
	}

	// Get the backend storage instance.
	st, err := flex.GetServices(c).StorageForStream(c, lst)
	if err != nil {
		return nil, nil, err
	}

	ch := make(chan logResp, maxBuffer)
	// Start fetching!
	go fetch(c, ch, st, lst, path)
	return ch, &logDesc{ls, lst}, nil
}

// fetchOnce does one backend storage request for logs, which may produce multiple log entries.
// Log entries are pushed into ch.
// Returns the next stream index to fetch.
// It is possible for the next index to be the terminal index + 1, so the calling
// code has to test for that.
func fetchOnce(
	c context.Context, ch chan<- logResp, st storage.Storage, path types.StreamPath,
	archived bool, index types.MessageIndex) (types.MessageIndex, error) {
	req := storage.GetRequest{
		Project: coordinator.Project(c),
		Path:    path,
		Index:   types.MessageIndex(index),
	}
	var ierr error
	// Get as many log entries as we can.
	// TODO(hinoka): We could use retry.Retry here, if the backend has a tendency to fail transiently.
	err := st.Get(c, req, func(e *storage.Entry) bool {
		var le *logpb.LogEntry
		if le, ierr = e.GetLogEntry(); ierr != nil {
			return false
		}
		sidx, _ := e.GetStreamIndex() // GetLogEntry succeeded, so this must.
		// Check for a non-continguous stream index.  This could mean a couple things:
		// Still streaming: The backend probably needs to catch up.  Return false and retry the fetch.
		// Archived: We permanently lost the log entries, just skip over them.
		if sidx != req.Index && !archived {
			return false
		}
		// Send the log entry over!  This may block if the channel buffer is full.
		ch <- logResp{le, nil}
		req.Index = sidx + 1
		return true
	})
	// TODO(hinoka): Handle not found case.
	if err == nil && ierr != nil {
		err = ierr
	}
	return req.Index, err
}

// fetch is a goroutine that fetches log entries from st at path and sends data back
// through ch.
// lst is used purely for checking Terminal Index.
func fetch(c context.Context, ch chan<- logResp, st storage.Storage, lst *coordinator.LogStreamState, path types.StreamPath) {
	defer close(ch)  // Close the channel when we're done fetching.
	defer st.Close() // Close the connection to the backend when we're done.
	index := types.MessageIndex(0)
	var err error
	for {
		// Get the terminal index from the LogStreamState again if we need to.
		// This is useful when the log stream terminated after the user request started.
		if lst.TerminalIndex == -1 {
			if err = datastore.Get(c, lst); err != nil {
				break
			}
		}

		// Do the actual work.
		if index, err = fetchOnce(c, ch, st, path, lst.ArchivalState().Archived(), index); err != nil {
			break
		}

		// Check if the log has finished streaming and if we're done.
		if 0 < lst.TerminalIndex && lst.TerminalIndex <= int64(index) {
			return
		}

		// Log is still streaming.  Sleep a bit and try again.
		// If the user cancelled the request, bail out instead.
		if tr := clock.Sleep(c, time.Second); tr.Err != nil {
			err = tr.Err
			break
		}
	}
	ch <- logResp{nil, err}
}

// writeOKHeaders writes the http response headers in accordence with the
// log stream type and user options.  The error is passed through.
// TODO(hinoka): Look at log stream and user options.
func writeOKHeaders(w http.ResponseWriter, desc *logDesc) {
	// Tell nginx not to buffer anything.
	w.Header().Set("X-Accel-Buffering", "no")
	// Tell the browser to prefer https.
	w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	// TODO(hinoka): This should be based on the stream type.
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
}

// writeErrHeaders writes the correct headers based on the error type.
// TODO(hinoka): Parse GRPC error codes and translate them to http codes.
func writeErrHeaders(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
}

// serve reads log entries from ch and writes into w.
func serve(ch <-chan logResp, w http.ResponseWriter) error {
	// Acquire the http flusher.  If the handler does not get a http.ResponseWriter
	// that implements http.Flusher, this will panic.  This is intentional.
	flusher := w.(http.Flusher)
	// Serve the logs.
	for logResp := range ch {
		log, err := logResp.log, logResp.err
		if err != nil {
			return err
		}
		for _, lines := range log.GetText().GetLines() {
			if lines == nil {
				return fmt.Errorf("could not print line")
			}
			fmt.Fprintf(w, "%s\n", lines.GetValue())
		}
		// Flush the ResponseWriter buffer, otherwise the client may never see anything
		// if there is less than 64Kb of log data sent.
		flusher.Flush()
	}
	return nil
}

// GetHandler is a HTTP handler for retrieving logs.
// Fetching happens concurrently with serving.
func GetHandler(ctx *router.Context) {
	// Start the fetcher and wait for fetched logs to arrive into ch.
	ch, desc, err := startFetch(ctx.Context, ctx.Params.ByName("path"))
	if err != nil {
		logging.WithError(err).Errorf(ctx.Context, "starting fetch")
		writeErrHeaders(ctx.Writer, err)
		return
	}
	writeOKHeaders(ctx.Writer, desc)

	// Write the log contents.
	if err := serve(ch, ctx.Writer); err != nil {
		http.Error(ctx.Writer, "LogDog encountered error while serving logs", http.StatusInternalServerError)
		logging.WithError(err).Errorf(ctx.Context, "serving logs")
		// TODO(hinoka): We can signal an error by acquiring the underlying TCP connection
		// using ctx.Writer.(http.Hijacker), and then sending a TCP RST packet, which
		// would manifest on the client side as "connection reset by peer".
	}
}
