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
	"errors"
	"fmt"
	"strings"

	v1 "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/server/router"
	"golang.org/x/net/context"
)

// maxBuffer specifies that maximum number of requests we can serve at a time
// before we start waiting.
var maxBuffer = 2

// fetch is a goroutine that fetches log chunks at path and sends data back
// through chLog.  Errors are pushed through chErr.  A nil error is pushed
// through chErr when fetch completes without error.
func fetch(c context.Context, chLog chan<- *v1.GetResponse, chErr chan<- error, project, path string) {
	server := GetServer(c)
	// Fetch logs until done.
	req := &v1.GetRequest{
		Project: project,
		Path:    path,
		State:   true,
		Index:   0,
	}
	for {
		resp, err := server.Get(c, req)
		if err != nil {
			chErr <- err
			return
		}
		if t := resp.GetDesc().GetStreamType(); t != logpb.StreamType_TEXT {
			chErr <- errors.New("Not a text stream")
		}
		chLog <- resp

		terminalIndex := resp.State.TerminalIndex
		lastLog := resp.Logs[len(resp.Logs)-1]
		lastIndex := int64(lastLog.StreamIndex)
		if lastIndex == terminalIndex {
			break
		}
		req.Index = lastIndex + 1
	}
	chErr <- nil
}

// HttpHandler is a plaintext HTTP handler for retrieving logs.
// Fetching happens concurrently with serving.
func HttpHandler(ctx *router.Context) {
	parts := strings.SplitN(ctx.Params.ByName("path"), "/", 3)
	if len(parts) != 3 {
		ctx.Writer.WriteHeader(400)
		fmt.Fprintf(ctx.Writer, "Missing path")
		return
	}
	first := true

	chLog := make(chan *v1.GetResponse, maxBuffer)
	chErr := make(chan error)
	// Start the fetcher and wait for fetched logs to arrive.
	go fetch(ctx.Context, chLog, chErr, parts[1], parts[2])
	for {
		select {
		case resp := <-chLog:
			if first {
				first = false
				ctx.Writer.WriteHeader(200)
				ctx.Writer.Header().Set("Content-Type", "text/plain")
			}
			for _, log := range resp.Logs {
				for _, lines := range log.GetText().GetLines() {
					fmt.Fprintf(ctx.Writer, "%s\n", lines.GetValue())
				}
			}

		case err := <-chErr:
			// Use this to signal done.
			if err != nil {
				ctx.Writer.WriteHeader(500)
				fmt.Fprintf(ctx.Writer, "Encountered error while loading: %s", err)
			}
			return
		}
	}
}
