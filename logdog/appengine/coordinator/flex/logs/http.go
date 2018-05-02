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

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/appengine/coordinator/flex"
	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/server/router"
	"golang.org/x/net/context"
)

// maxBuffer specifies that maximum number of log entries we can buffer at a time.
var maxBuffer = 1000

// logResp is the response object passed from the fetch routine back to the rendering routine.
type logResp struct {
	log *logpb.LogEntry
	err error
}

// getState returns the state of a log stream.
func getState(c context.Context, path types.StreamPath) (*coordinator.LogStreamState, error) {
	ls := &coordinator.LogStream{ID: coordinator.LogStreamID(path)}
	lst := ls.State(c)
	return lst, datastore.Get(c, lst)
}

// checkPurged checks to see if the log is purged.
func checkPurged(c context.Context, path types.StreamPath) error {
	ls := &coordinator.LogStream{ID: coordinator.LogStreamID(path)}
	if err := datastore.Get(c, ls); err != nil {
		return err
	}

	// If this log entry is Purged and we're not admin, pretend it doesn't exist.
	if ls.Purged {
		if authErr := coordinator.IsAdminUser(c); authErr != nil {
			return grpcutil.Errf(codes.NotFound, "path not found")
		}
	}
	return nil
}

// getStorage returns the storage backend of a log stream.
// This also bails out if the log has been purged.
func getStorage(c context.Context, path types.StreamPath) (storage.Storage, error) {
	lst, err := getState(c, path)
	if err != nil {
		return nil, err
	}
	return flex.GetServices(c).StorageForStream(c, lst)
}

// getTerminalIndex returns the terminal index for a log stream, if available
// (I.E. the log has finished streaming).
func getTerminalIndex(c context.Context, path types.StreamPath) (int64, error) {
	lst, err := getState(c, path)
	if err != nil {
		return -1, err
	}
	return lst.TerminalIndex, nil
}

// startFetch sets up the backend storage instance (Either BigTable or Google Storage)
// and then jump starts the goroutine to fetch logs from the backend storage.
// It returns a channel where log entires are sent back.
func startFetch(c context.Context, path types.StreamPath) (<-chan logResp, error) {
	st, err := getStorage(c, path)
	if err != nil {
		return nil, err
	}
	defer st.Close()
	ch := make(chan logResp, maxBuffer)
	go fetch(c, ch, st, path)
	return ch, nil
}

// fetch is a goroutine that fetches log entries at path and sends data back
// through ch.
func fetch(c context.Context, ch chan<- logResp, st storage.Storage, path types.StreamPath) {
	defer close(ch)
	req := storage.GetRequest{
		Project: coordinator.Project(c),
		Path:    path,
		Index:   types.MessageIndex(0),
	}
	terminalIndex := int64(-1)
	var err, ierr error
	for {
		// Get the terminal index if we need to.
		// This may return -1 again if the log is still streaming, which is fine.
		if terminalIndex == -1 {
			terminalIndex, err = getTerminalIndex(c, path)
			if err != nil {
				break
			}
		}

		// Get as many log entries as we can.
		err = st.Get(c, req, func(e *storage.Entry) bool {
			var le *logpb.LogEntry
			if le, ierr = e.GetLogEntry(); ierr != nil {
				return false
			}
			sidx, _ := e.GetStreamIndex() // GetLogEntry succeeded, so this must.
			// We got a non-continguous stream.  Lets wait for the backend to catch up,
			// and then try again.
			if sidx != req.Index {
				return false
			}
			// Send the log entry over!
			ch <- logResp{le, nil}
			req.Index++
			return true
		})
		// TODO: Handle not found case.
		switch {
		case err == nil && ierr != nil:
			err = ierr
			fallthrough
		case err != nil:
			break
		}

		// Check if the log has finished streaming and if we're done.
		if 0 < terminalIndex && terminalIndex <= int64(req.Index) {
			break
		}
		// Log is still streaming.  Sleep a bit and try again.
		time.Sleep(time.Second)
	}
	ch <- logResp{nil, err}
}

// parse parses a log stream path and returns the internal path structs, if valid.
func parse(pathStr string) (project types.ProjectName, path types.StreamPath, err error) {
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

// HttpHandler is a plaintext HTTP handler for retrieving logs.
// Fetching happens concurrently with serving.
func HttpHandler(ctx *router.Context) {
	project, path, err := parse(ctx.Params.ByName("path"))
	if err != nil {
		ctx.Writer.WriteHeader(400)
		fmt.Fprintf(ctx.Writer, err.Error())
		return
	}

	// Perform the ACL check, this also installs the project name into the context.
	if err := coordinator.WithProjectNamespace(&ctx.Context, project, coordinator.NamespaceAccessREAD); err != nil {
		// TODO: Translate GRPC code to HTTP codes.
		ctx.Writer.WriteHeader(500)
		fmt.Fprintf(ctx.Writer, "Error: %s", err)
		return
	}

	// Start the fetcher and wait for fetched logs to arrive into ch.
	ch, err := startFetch(ctx.Context, path)
	if err != nil {
		ctx.Writer.WriteHeader(500)
		fmt.Fprintf(ctx.Writer, err.Error())
		return
	}

	// Serve the logs.
	first := true
	for logResp := range ch {
		log, err := logResp.log, logResp.err
		switch {
		case log == nil && err == nil:
			return // Done!
		case err != nil:
			ctx.Writer.WriteHeader(500)
			fmt.Fprintf(ctx.Writer, "Encountered error while loading: %s", err)
			return
		case log != nil:
			if first {
				first = false
				ctx.Writer.Header().Set("X-Accel-Buffering", "no")
				ctx.Writer.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
				ctx.Writer.Header().Set("Content-Type", "text/plain")
				ctx.Writer.WriteHeader(200)
			}
			for _, lines := range log.GetText().GetLines() {
				fmt.Fprintf(ctx.Writer, "%s\n", lines.GetValue())
			}
			// Flush the ResponseWriter buffer, otherwise the client may never see anything
			// if there is less than 64Kb of log data sent.
			if f, ok := ctx.Writer.(http.Flusher); ok {
				f.Flush()
			}
		default:
			panic("this should never happen")
		}
	}
}
