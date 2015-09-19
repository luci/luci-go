// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This file contains types which are mirrors/duplicates of the upstream SDK
// types. This exists so that users can depend solely on this wrapper library
// without necessarially needing an SDK implementation present.
//
// This was done (instead of type-aliasing from the github version of the SDK)
// because some of the types need to be tweaked (like Task.RetryOptions) to
// interact well with the wrapper, and the inconsistency of having some types
// defined by the gae package and others defined by the SDK was pretty awkward.

package taskqueue

import (
	"net/http"
	"time"
)

// Statistics represents statistics about a single task queue.
type Statistics struct {
	Tasks     int       // may be an approximation
	OldestETA time.Time // zero if there are no pending tasks

	Executed1Minute int     // tasks executed in the last minute
	InFlight        int     // tasks executing now
	EnforcedRate    float64 // requests per second
}

// RetryOptions let you control whether to retry a task and the backoff intervals between tries.
type RetryOptions struct {
	// Number of tries/leases after which the task fails permanently and is deleted.
	// If AgeLimit is also set, both limits must be exceeded for the task to fail permanently.
	RetryLimit int32

	// Maximum time allowed since the task's first try before the task fails permanently and is deleted (only for push tasks).
	// If RetryLimit is also set, both limits must be exceeded for the task to fail permanently.
	AgeLimit time.Duration

	// Minimum time between successive tries (only for push tasks).
	MinBackoff time.Duration

	// Maximum time between successive tries (only for push tasks).
	MaxBackoff time.Duration

	// Maximum number of times to double the interval between successive tries before the intervals increase linearly (only for push tasks).
	MaxDoublings int32

	// If MaxDoublings is zero, set ApplyZeroMaxDoublings to true to override the default non-zero value.
	// Otherwise a zero MaxDoublings is ignored and the default is used.
	ApplyZeroMaxDoublings bool
}

// Task represents a taskqueue task to be executed.
type Task struct {
	// Path is the worker URL for the task.
	// If unset, it will default to /_ah/queue/<queue_name>.
	Path string

	// Payload is the data for the task.
	// This will be delivered as the HTTP request body.
	// It is only used when Method is POST, PUT or PULL.
	// url.Values' Encode method may be used to generate this for POST requests.
	Payload []byte

	// Additional HTTP headers to pass at the task's execution time.
	// To schedule the task to be run with an alternate app version
	// or backend, set the "Host" header.
	Header http.Header

	// Method is the HTTP method for the task ("GET", "POST", etc.),
	// or "PULL" if this is task is destined for a pull-based queue.
	// If empty, this defaults to "POST".
	Method string

	// A name for the task.
	// If empty, a name will be chosen.
	Name string

	// Delay specifies the duration the task queue service must wait
	// before executing the task.
	// Either Delay or ETA may be set, but not both.
	Delay time.Duration

	// ETA specifies the earliest time a task may be executed (push queues)
	// or leased (pull queues).
	// Either Delay or ETA may be set, but not both.
	ETA time.Time

	// The number of times the task has been dispatched or leased.
	RetryCount int32

	// Tag for the task. Only used when Method is PULL.
	Tag string

	// Retry options for this task. May be nil.
	RetryOptions *RetryOptions
}

// Duplicate returns a deep copy of this Task.
func (t *Task) Duplicate() *Task {
	ret := *t

	if len(t.Header) > 0 {
		ret.Header = make(http.Header, len(t.Header))
		for k, vs := range t.Header {
			newVs := make([]string, len(vs))
			copy(newVs, vs)
			ret.Header[k] = newVs
		}
	}

	if len(t.Payload) > 0 {
		ret.Payload = make([]byte, len(t.Payload))
		copy(ret.Payload, t.Payload)
	}

	if t.RetryOptions != nil {
		ret.RetryOptions = &RetryOptions{}
		*ret.RetryOptions = *t.RetryOptions
	}

	return &ret
}
