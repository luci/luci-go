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

func (b *Backend) multiTask(c context.Context, q string, f func(chan<- *tq.Task)) (int, error) {
	batch := b.multiTaskBatchSize
	if batch <= 0 {
		batch = defaultMultiTaskBatchSize
	}

	ti := tq.Get(c)
	send := func(tasks []*tq.Task) int {
		sent := len(tasks)
		if sent == 0 {
			return 0
		}

		// Add the tasks. If an error occurs, log each specific error.
		if err := errors.Filter(ti.AddMulti(tasks, q), tq.ErrTaskAlreadyAdded); err != nil {
			switch t := err.(type) {
			case errors.MultiError:
				// Some tasks failed to be added.
				for i, e := range t {
					if e != nil {
						log.Fields{
							log.ErrorKey: e,
							"index":      i,
							"taskPath":   tasks[i].Path,
							"taskParams": string(tasks[i].Payload),
						}.Errorf(c, "Failed to add task queue task.")
						sent--
					}
				}

			default:
				// General AddMulti error.
				log.WithError(t).Errorf(c, "Failed to add task queue tasks.")
				return 0
			}
		}

		return sent
	}

	// Run our generator function in a separate goroutine.
	taskC := make(chan *tq.Task, batch)
	go func() {
		defer close(taskC)
		f(taskC)
	}()

	// Pull tasks from our task channel and dispatch them in batches via send.
	tasks := make([]*tq.Task, 0, batch)
	var total, numSent int
	for t := range taskC {
		total++

		tasks = append(tasks, t)
		if len(tasks) >= batch {
			numSent += send(tasks)
			tasks = tasks[:0]
		}

	}

	// Final send, in case a not-full batch of tasks built up.
	numSent += send(tasks)

	if numSent != total {
		log.Fields{
			"total": total,
			"added": numSent,
		}.Errorf(c, "Not all tasks could be added.")
		return numSent, errors.New("error adding tasks")
	}
	return numSent, nil
}
