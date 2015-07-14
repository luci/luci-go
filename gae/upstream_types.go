// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This file contains types which are mirrors/duplicates of the upstream SDK
// types. This exists so that users can depend solely on this wrapper library
// without necessarially needing an SDK implementation present.
//
// This was done (instead of type-aliasing from the github version of the SDK)
// because some of the types need to be tweaked (like TQTask.RetryOptions) to
// interact well with the wrapper, and the inconsistency of having some types
// defined by the gae package and others defined by the SDK was pretty awkward.

package gae

import (
	"net/http"
	"time"
)

// DSByteString is a short byte slice (up to 1500 bytes) that can be indexed.
type DSByteString []byte

// BSKey is a key for a blobstore blob.
//
// Conceptually, this type belongs in the blobstore package, but it lives in the
// appengine package to avoid a circular dependency: blobstore depends on
// datastore, and datastore needs to refer to the BSKey type.
//
// Blobstore is NOT YET supported by gae, but may be supported later. Its
// inclusion here is so that the RawDatastore can interact (and round-trip)
// correctly with other datastore API implementations.
type BSKey string

// DSGeoPoint represents a location as latitude/longitude in degrees.
//
// You probably shouldn't use these, but their inclusion here is so that the
// RawDatastore can interact (and round-trip) correctly with other datastore API
// implementations.
type DSGeoPoint struct {
	Lat, Lng float64
}

// Valid returns whether a DSGeoPoint is within [-90, 90] latitude and [-180,
// 180] longitude.
func (g DSGeoPoint) Valid() bool {
	return -90 <= g.Lat && g.Lat <= 90 && -180 <= g.Lng && g.Lng <= 180
}

// DSTransactionOptions are the options for running a transaction.
type DSTransactionOptions struct {
	// XG is whether the transaction can cross multiple entity groups. In
	// comparison, a single group transaction is one where all datastore keys
	// used have the same root key. Note that cross group transactions do not
	// have the same behavior as single group transactions. In particular, it
	// is much more likely to see partially applied transactions in different
	// entity groups, in global queries.
	// It is valid to set XG to true even if the transaction is within a
	// single entity group.
	XG bool
}

// MCStatistics represents a set of statistics about the memcache cache.  This
// may include items that have expired but have not yet been removed from the
// cache.
type MCStatistics struct {
	Hits     uint64 // Counter of cache hits
	Misses   uint64 // Counter of cache misses
	ByteHits uint64 // Counter of bytes transferred for gets

	Items uint64 // Items currently in the cache
	Bytes uint64 // Size of all items currently in the cache

	Oldest int64 // Age of access of the oldest item, in seconds
}

// TQStatistics represents statistics about a single task queue.
type TQStatistics struct {
	Tasks     int       // may be an approximation
	OldestETA time.Time // zero if there are no pending tasks

	Executed1Minute int     // tasks executed in the last minute
	InFlight        int     // tasks executing now
	EnforcedRate    float64 // requests per second
}

// TQRetryOptions let you control whether to retry a task and the backoff intervals between tries.
type TQRetryOptions struct {
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

// TQTask represents a taskqueue task to be executed.
type TQTask struct {
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
	RetryOptions *TQRetryOptions
}

// GICertificate represents a public certificate for the app.
type GICertificate struct {
	KeyName string
	Data    []byte // PEM-encoded X.509 certificate
}
