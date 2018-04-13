package main

import (
	"errors"
	"fmt"
	"strings"

	v1 "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/appengine/coordinator/flex/logs"
	"go.chromium.org/luci/server/router"
	"golang.org/x/net/context"
)

// maxBuffer specifies that maximum number of requests we can serve at a time
// before we start waiting.
var maxBuffer = 2

func fetch(c context.Context, chLog chan<- *v1.GetResponse, chErr chan<- error, project, path string) {
	server := logs.New()
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
		if int64(lastLog.StreamIndex) == terminalIndex {
			break
		}
	}
	chErr <- nil
}

func httpHandler(ctx *router.Context) {
	parts := strings.SplitN(ctx.Params.ByName("path"), "/", 3)
	if len(parts) != 3 {
		ctx.Writer.WriteHeader(400)
		fmt.Fprintf(ctx.Writer, "Missing path")
		return
	}
	first := true

	chLog := make(chan *v1.GetResponse, maxBuffer)
	chErr := make(chan error)
	go fetch(ctx.Context, chLog, chErr, parts[1], parts[2])
	for {
		select {
		case resp := <-chLog:
			if first {
				first = false
				ctx.Writer.WriteHeader(200)
				fmt.Fprintf(ctx.Writer, "Found log!\n")
				fmt.Fprintf(ctx.Writer, "Archive Info:\n%s\n", resp.State.Archive)
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
