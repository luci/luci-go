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
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"

	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"

	"go.chromium.org/luci/common/logging"
)

// LoopbackHTTPExecutor is an Executor that executes tasks by calling the given
// HTTP handler.
type LoopbackHTTPExecutor struct {
	Handler http.Handler
}

// Execute dispatches the task to the HTTP handler in a dedicated goroutine.
//
// Marks the task as failed if the response status code is outside of
// range [200-299].
func (e *LoopbackHTTPExecutor) Execute(ctx context.Context, t *Task, done func(retry bool)) {
	success := false
	defer func() {
		done(!success)
	}()

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

	// Make the URL relative.
	parsedURL.Scheme = ""
	parsedURL.Host = ""
	requestURL = parsedURL.String()

	req := httptest.NewRequest(method.String(), requestURL, bytes.NewReader(body))
	req.Host = host
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// See https://cloud.google.com/tasks/docs/creating-http-target-tasks#handler
	// We emulate only headers we actually use.
	req.Header.Set("X-CloudTasks-TaskExecutionCount", strconv.Itoa(t.Attempts-1))
	if t.Attempts > 1 {
		req.Header.Set("X-CloudTasks-TaskRetryReason", "task handler failed")
	}

	rr := httptest.NewRecorder()
	e.Handler.ServeHTTP(rr, req)
	status := rr.Result().StatusCode
	success = status >= 200 && status <= 299
}
