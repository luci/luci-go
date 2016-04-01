// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package backend

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/golang/protobuf/proto"
	tq "github.com/luci/gae/service/taskqueue"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

// httpError
type httpError struct {
	reason error
	code   int
}

func (e *httpError) Error() string {
	if r := e.reason; r != nil {
		return fmt.Sprintf("%v: %q", e.reason, http.StatusText(e.code))
	}
	return http.StatusText(e.code)
}

func newHTTPError(reason error, code int) error {
	return &httpError{reason, code}
}

// errorWrapper wraps an error-returning function and responds with an
// InternalServerError if an error is returned.
func errorWrapper(c context.Context, w http.ResponseWriter, f func() error) {
	if err := f(); err != nil {
		statusCode := http.StatusInternalServerError
		if e, ok := err.(*httpError); ok {
			statusCode = e.code
		}

		log.Fields{
			log.ErrorKey: err,
			"statusCode": statusCode,
		}.Errorf(c, "Backend handler returned error.")
		w.WriteHeader(statusCode)
	}
}

func mkValues(params map[string]string) url.Values {
	values := make(url.Values, len(params))
	for k, v := range params {
		values[k] = []string{v}
	}
	return values
}

func createTask(path string, params map[string]string) *tq.Task {
	h := make(http.Header)
	h.Set("Content-Type", "application/x-www-form-urlencoded")
	return &tq.Task{
		Path:    path,
		Header:  h,
		Payload: []byte(mkValues(params).Encode()),
		Method:  "POST",
	}
}

// createPullTask is a generic pull queue task creation method. It is used to
// instantiate pull queue tasks.
func createPullTask(msg proto.Message) (*tq.Task, error) {
	t := tq.Task{
		Method: "PULL",
	}

	if msg != nil {
		var err error
		t.Payload, err = proto.Marshal(msg)
		if err != nil {
			return nil, err
		}
	}
	return &t, nil
}
