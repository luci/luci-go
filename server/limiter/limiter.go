// Copyright 2019 The LUCI Authors.
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

package limiter

import (
	"context"
	"sync/atomic"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
)

// ErrLimitReached is returned by CheckRequest when some limit is reached.
var ErrLimitReached = errors.New("a server limit reached")

var (
	// Number of in-flight requests.
	concurrencyCurGauge = metric.NewInt(
		"server/limiter/concurrency/cur",
		"Number of requests being processed right now.",
		nil,
		field.String("limiter"), // name of the limiter that reports the metric
	)

	// Configured maximum number of in-flight requests.
	concurrencyMaxGauge = metric.NewInt(
		"server/limiter/concurrency/max",
		"A configured limit on a number of concurrently processed requests.",
		nil,
		field.String("limiter"), // name of the limiter that reports the metric
	)

	// Counter with rejected requests.
	rejectedCounter = metric.NewCounter(
		"server/limiter/rejected",
		"Number of rejected requests.",
		nil,
		field.String("limiter"), // name of the limiter that did the rejection
		field.String("call"),    // an RPC or an endpoint being called (if known)
		field.String("peer"),    // who's making the request (if known), see also peer.go.
		field.String("reason"))  // why the request was rejected
)

// Options contains overall configuration of a Limiter instance.
type Options struct {
	Name                  string // used for metric fields and for logs
	AdvisoryMode          bool   // if true, don't actually reject requests, just log
	MaxConcurrentRequests int64  // a hard limit on a number of concurrent requests
}

// Limiter is a stateful runtime object that decides whether to accept or reject
// requests based on the current load (calculated from requests that went
// through it).
//
// It is also responsible for maintaining monitoring metrics that describe its
// state and what request it rejected.
//
// May be running in advisory mode, in which it will do all the usual processing
// and logging, but won't actually reject the requests.
//
// All methods are safe for concurrent use.
type Limiter struct {
	opts        Options // options passed to New, as is
	concurrency int64   // atomic int with number of current in-flight requests
}

// RequestInfo holds information about a single inbound request.
//
// Used by the limiter to decide whether to accept or reject the request.
//
// Pretty sparse now, but in the future will contain fields like cost, a QoS
// class and an attempt count, which will help the limiter to decide what
// requests to drop.
//
// Fields `CallLabel` and `PeerLabel` are intentionally pretty generic, since
// they will be used only as labels in internal maps and metric fields. Their
// internal structure and meaning are not important to the limiter, but the
// cardinality of the set of their possible values must be reasonably bounded.
type RequestInfo struct {
	CallLabel string // an RPC or an endpoint being called (if known)
	PeerLabel string // who's making the request (if known), see also peer.go
}

// New returns a new limiter.
//
// Returns an error if options are invalid.
func New(opts Options) (*Limiter, error) {
	if opts.MaxConcurrentRequests <= 0 {
		return nil, errors.New("max concurrent requests must be positive")
	}
	return &Limiter{opts: opts}, nil
}

// CheckRequest is called before processing a request.
//
// If it returns an error, the request should be declined as soon as possible
// with Unavailable/HTTP 503 status and the given error (which is an annotated
// ErrLimitReached).
//
// If it succeeds, the request should be processed as usual, and the returned
// callback called afterwards to notify the limiter the processing is done.
func (l *Limiter) CheckRequest(ctx context.Context, ri *RequestInfo) (done func(), err error) {
	// TODO(vadimsh): This is the simplest limiter implementation possible. It
	// will likely learn more tricks once we understand what features we need.
	for {
		cur := atomic.LoadInt64(&l.concurrency)
		if cur >= l.opts.MaxConcurrentRequests {
			return l.reject(ctx, ri, "max concurrency")
		}
		if !atomic.CompareAndSwapInt64(&l.concurrency, cur, cur+1) {
			continue // race, try again
		}
		l.updateConcurrencyMetric(ctx, cur+1)
		return func() {
			cur := atomic.AddInt64(&l.concurrency, -1)
			l.updateConcurrencyMetric(ctx, cur)
		}, nil
	}
}

// reject is called when the request is rejected.
//
// It updates metrics and logs and either returns an error (in non-advisory
// mode) or a noop-callback (in advisory mode).
func (l *Limiter) reject(ctx context.Context, ri *RequestInfo, reason string) (done func(), err error) {
	rejectedCounter.Add(ctx, 1, l.opts.Name, ri.CallLabel, ri.PeerLabel, reason)
	if l.opts.AdvisoryMode {
		logging.Warningf(ctx, "limiter advisory mode: the request hit the %s limit", reason)
		return func() {}, nil // allow the request to happen
	}
	logging.Errorf(ctx, "limiter: the request hit the %s limit", reason)
	return nil, errors.Annotate(ErrLimitReached, "hit the %s limit", reason).Err()
}

// updateConcurrencyMetric is called when the number of in-flight request
// changes.
func (l *Limiter) updateConcurrencyMetric(ctx context.Context, cur int64) {
	concurrencyCurGauge.Set(ctx, cur, l.opts.Name)
	concurrencyMaxGauge.Set(ctx, l.opts.MaxConcurrentRequests, l.opts.Name)
}
