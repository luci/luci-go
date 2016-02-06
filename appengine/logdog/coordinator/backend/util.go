// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package backend

import (
	"fmt"
	"net/http"
	"net/url"

	tq "github.com/luci/gae/service/taskqueue"
	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

const (
	// defaultMultiTaskBatchSize is the default value for Backend's
	// multiTaskBatchSize parameter.
	defaultMultiTaskBatchSize = 100
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

func (b *Backend) multiTask(c context.Context, q string, f func(chan<- *tq.Task)) (int, error) {
	batch := b.multiTaskBatchSize
	if batch <= 0 {
		batch = defaultMultiTaskBatchSize
	}

	ti := tq.Get(c)
	send := func(tasks []*tq.Task) errors.MultiError {
		if len(tasks) == 0 {
			return nil
		}

		// Add the tasks. If an error occurs, log each specific error, then
		var merr errors.MultiError
		err := ti.AddMulti(tasks, q)
		if err != nil {
			switch t := err.(type) {
			case errors.MultiError:
				for _, e := range t {
					switch e {
					case tq.ErrTaskAlreadyAdded:
						break

					default:
						merr = append(merr, t...)
					}
				}

			default:
				log.WithError(t).Errorf(c, "Failed to add tasks.")
				merr = append(merr, t)
			}
		}
		return merr
	}

	count := 0
	taskC := make(chan *tq.Task)
	errC := make(chan errors.MultiError)
	go func() {
		var merr errors.MultiError
		defer func() {
			errC <- merr
			close(errC)
		}()

		tasks := make([]*tq.Task, 0, batch)
		for t := range taskC {
			count++

			tasks = append(tasks, t)
			if len(tasks) >= batch {
				if err := send(tasks); err != nil {
					merr = append(merr, err...)
				}
				tasks = tasks[:0]
			}
		}

		// Final send, in case a not-full batch of tasks built up.
		merr = append(merr, send(tasks)...)
		return
	}()

	func() {
		defer close(taskC)
		f(taskC)
	}()

	merr := <-errC
	for _, e := range merr {
		log.WithError(e).Errorf(c, "Failed to add task.")
	}
	count -= len(merr)
	return count, errors.SingleError(merr)
}
