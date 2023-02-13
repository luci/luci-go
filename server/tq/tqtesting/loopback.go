// Copyright 2020 The LUCI Authors.
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

package tqtesting

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"

	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"

	"go.chromium.org/luci/common/logging"
)

// LoopbackHTTPExecutor is an Executor that executes tasks by sending HTTP
// requests to the server with TQ module serving at the given (usually loopback)
// address.
//
// Used exclusively when running TQ locally.
type LoopbackHTTPExecutor struct {
	// ServerAddr is where the server is listening for requests.
	ServerAddr net.Addr
}

// Execute dispatches the task to the HTTP handler in a dedicated goroutine.
//
// Marks the task as failed if the response status code is outside of
// range [200-299].
func (e *LoopbackHTTPExecutor) Execute(ctx context.Context, t *Task, done func(retry bool)) {
	if t.Message != nil {
		done(false)
		panic("Executing PubSub tasks is not supported yet") // break tests loudly
	}

	success := false
	defer func() {
		done(!success)
	}()

	if e.ServerAddr == nil {
		logging.Errorf(ctx, "LoopbackHTTPExecutor is not configured. Is the server exposing main HTTP port?")
		return
	}

	var method taskspb.HttpMethod
	var requestURL string
	var headers map[string]string
	var body []byte

	switch mt := t.Task.MessageType.(type) {
	case *taskspb.Task_HttpRequest:
		method = mt.HttpRequest.HttpMethod
		requestURL = mt.HttpRequest.Url
		headers = mt.HttpRequest.Headers
		body = mt.HttpRequest.Body
	case *taskspb.Task_AppEngineHttpRequest:
		method = mt.AppEngineHttpRequest.HttpMethod
		requestURL = mt.AppEngineHttpRequest.RelativeUri
		headers = mt.AppEngineHttpRequest.Headers
		body = mt.AppEngineHttpRequest.Body
	default:
		logging.Errorf(ctx, "Bad task, no payload: %q", t.Task)
		return
	}

	parsedURL, err := url.Parse(requestURL)
	if err != nil {
		logging.Errorf(ctx, "Bad task URL %q", requestURL)
		return
	}
	host := parsedURL.Host

	// Make the URL relative to the localhost server at the requested port.
	parsedURL.Scheme = "http"
	parsedURL.Host = e.ServerAddr.String() // this is "<host>:<port>"
	requestURL = parsedURL.String()

	req, err := http.NewRequestWithContext(ctx, method.String(), requestURL, bytes.NewReader(body))
	if err != nil {
		logging.Errorf(ctx, "Could not construct HTTP request: %s", err)
		return
	}
	req.Host = host // sets "Host" request header
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// See https://cloud.google.com/tasks/docs/creating-http-target-tasks#handler
	// We emulate only headers we actually use.
	req.Header.Set("X-CloudTasks-TaskExecutionCount", strconv.Itoa(t.Attempts-1))
	if t.Attempts > 1 {
		req.Header.Set("X-CloudTasks-TaskRetryReason", "task handler failed")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logging.Errorf(ctx, "Failed to send HTTP request: %s", err)
		return
	}
	defer resp.Body.Close()
	// Read the body fully to be able to reuse the connection.
	if _, err = io.Copy(io.Discard, resp.Body); err != nil {
		logging.Errorf(ctx, "Failed to read the response: %s", err)
		return
	}

	success = resp.StatusCode >= 200 && resp.StatusCode <= 299
}
