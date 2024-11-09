// Copyright 2015 The LUCI Authors.
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

// Package urlfetch implements tasks that just make HTTP calls.
package urlfetch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/api/pubsub/v1"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/urlfetch"

	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
)

// TaskManager implements task.Manager interface for tasks defined with
// UrlFetchTask proto message
type TaskManager struct {
}

// Name is part of Manager interface.
func (m TaskManager) Name() string {
	return "url_fetch"
}

// ProtoMessageType is part of Manager interface.
func (m TaskManager) ProtoMessageType() proto.Message {
	return (*messages.UrlFetchTask)(nil)
}

// Traits is part of Manager interface.
func (m TaskManager) Traits() task.Traits {
	return task.Traits{
		Multistage: false, // we don't use task.StatusRunning state
	}
}

// ValidateProtoMessage is part of Manager interface.
func (m TaskManager) ValidateProtoMessage(c *validation.Context, msg proto.Message, realmID string) {
	cfg, ok := msg.(*messages.UrlFetchTask)
	if !ok {
		c.Errorf("wrong type %T, expecting *messages.UrlFetchTask", msg)
		return
	}
	if cfg == nil {
		c.Errorf("expecting a non-empty UrlFetchTask")
		return
	}

	// Validate 'method' field.
	// TODO(vadimsh): Add more methods (POST, PUT) when 'Body' is added.
	goodMethods := map[string]bool{"GET": true}
	if cfg.Method != "" && !goodMethods[cfg.Method] {
		c.Errorf("unsupported HTTP method: %q", cfg.Method)
	}

	// Validate 'url' field.
	if cfg.Url == "" {
		c.Errorf("field 'url' is required")
	} else {
		u, err := url.Parse(cfg.Url)
		if err != nil {
			c.Errorf("invalid URL %q: %s", cfg.Url, err)
		} else if !u.IsAbs() {
			c.Errorf("not an absolute url: %q", cfg.Url)
		}
	}

	// Validate 'timeout_sec' field. GAE task queue request deadline is 10 min, so
	// limit URL fetch call duration to 8 min (giving 2 min to spare).
	if cfg.TimeoutSec != 0 {
		if cfg.TimeoutSec < 1 {
			c.Errorf("minimum allowed 'timeout_sec' is 1 sec, got %d", cfg.TimeoutSec)
		}
		if cfg.TimeoutSec > 480 {
			c.Errorf("maximum allowed 'timeout_sec' is 480 sec, got %d", cfg.TimeoutSec)
		}
	}
}

// LaunchTask is part of Manager interface.
func (m TaskManager) LaunchTask(c context.Context, ctl task.Controller) error {
	cfg := ctl.Task().(*messages.UrlFetchTask)
	started := clock.Now(c)

	// Defaults.
	method := cfg.Method
	if method == "" {
		method = "GET"
	}
	timeout := cfg.TimeoutSec
	if timeout == 0 {
		timeout = 60
	}

	ctl.DebugLog("%s %s", method, cfg.Url)

	// There must be no errors here in reality, since cfg is validated already by
	// ValidateProtoMessage.
	u, err := url.Parse(cfg.Url)
	if err != nil {
		return err
	}

	type tuple struct {
		resp *http.Response
		body []byte // first 4Kb of the response, for debug log
		err  error
	}
	result := make(chan tuple)

	// Do the fetch asynchronously with datastore update.
	go func() {
		defer close(result)
		c, cancel := clock.WithTimeout(c, time.Duration(timeout)*time.Second)
		defer cancel()
		client := &http.Client{Transport: urlfetch.Get(c)}
		resp, err := client.Do(&http.Request{
			Method: method,
			URL:    u,
		})
		if err != nil {
			result <- tuple{nil, nil, err}
			return
		}
		defer resp.Body.Close()
		// Ignore read errors here. HTTP status code is set, it's the main output
		// of the operation. Read 4K only since we use body only for debug message
		// that is limited in size.
		buf := bytes.Buffer{}
		io.CopyN(&buf, resp.Body, 4096)
		result <- tuple{resp, buf.Bytes(), nil}
	}()

	// Save the invocation log now (since URL fetch can take up to 8 minutes).
	// Ignore errors. As long as final Save is OK, we don't care about this one.
	// Do NOT set status to StatusRunning, because by doing so we take
	// responsibility to detect crashes below and we don't want to do it (let
	// the scheduler retry LaunchTask automatically instead).
	if err := ctl.Save(c); err != nil {
		logging.Warningf(c, "Failed to save invocation state: %s", err)
	}

	// Wait for completion.
	res := <-result

	duration := clock.Now(c).Sub(started)
	status := task.StatusSucceeded
	if res.err != nil || res.resp.StatusCode >= 400 {
		status = task.StatusFailed
	}

	ctl.DebugLog("Finished with overall status %s in %s", status, duration)
	if res.err != nil {
		ctl.DebugLog("URL fetch error: %s", res.err)
	} else {
		ctl.DebugLog(dumpResponse(res.resp, res.body))
	}
	ctl.State().Status = status
	return nil
}

// AbortTask is part of Manager interface.
func (m TaskManager) AbortTask(c context.Context, ctl task.Controller) error {
	return nil
}

// ExamineNotification is part of Manager interface.
func (m TaskManager) ExamineNotification(c context.Context, msg *pubsub.PubsubMessage) string {
	return ""
}

// HandleNotification is part of Manager interface.
func (m TaskManager) HandleNotification(c context.Context, ctl task.Controller, msg *pubsub.PubsubMessage) error {
	return errors.New("not implemented")
}

// HandleTimer is part of Manager interface.
func (m TaskManager) HandleTimer(c context.Context, ctl task.Controller, name string, payload []byte) error {
	return errors.New("not implemented")
}

// GetDebugState is part of Manager interface.
func (m TaskManager) GetDebugState(c context.Context, ctl task.ControllerReadOnly) (*internal.DebugManagerState, error) {
	return nil, fmt.Errorf("no debug state")
}

////////////////////////////////////////////////////////////////////////////////

// dumpResponse converts http.Response to text for the invocation debug log.
func dumpResponse(resp *http.Response, body []byte) string {
	out := &bytes.Buffer{}
	fmt.Fprintln(out, resp.Status)
	resp.Header.Write(out)
	fmt.Fprintln(out)
	if len(body) == 0 {
		fmt.Fprintln(out, "<empty body>")
	} else if isTextContent(resp.Header) {
		out.Write(body)
		if body[len(body)-1] != '\n' {
			fmt.Fprintln(out)
		}
		if int64(len(body)) < resp.ContentLength {
			fmt.Fprintln(out, "<truncated>")
		}
	} else {
		fmt.Fprintln(out, "<binary response>")
	}
	return out.String()
}

var textContentTypes = []string{
	"text/",
	"application/json",
	"application/xml",
}

// isTextContent returns True if Content-Type header corresponds to some
// readable text type.
func isTextContent(h http.Header) bool {
	for _, header := range h["Content-Type"] {
		for _, good := range textContentTypes {
			if strings.HasPrefix(header, good) {
				return true
			}
		}
	}
	return false
}
