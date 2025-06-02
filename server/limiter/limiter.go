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

package limiter

import (
	"context"
	"fmt"
	"sync/atomic"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
)

// ErrLimitReached is returned by CheckRequest when some limit is reached.
var ErrLimitReached = errors.New("the server limit reached")

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

// Options contains configuration of a single Limiter instance.
type Options struct {
	Name                  string // used for metric fields, logs and error messages
	AdvisoryMode          bool   // if true, don't actually reject requests, just log
	MaxConcurrentRequests int64  // a hard limit on a number of concurrent requests
}

// Limiter is a stateful runtime object that decides whether to accept or reject
// requests based on the current load (calculated from requests that went
// through it).
//
// It is also responsible for maintaining monitoring metrics that describe its
// state and what requests it rejects.
//
// May be running in advisory mode, in which it will do all the usual processing
// and logging, but won't actually reject the requests.
//
// All methods are safe for concurrent use.
type Limiter struct {
	opts        Options // options passed to New, as is
	titleForLog string  // how the limiter is named in logs and error replies
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
	return &Limiter{
		opts:        opts,
		titleForLog: fmt.Sprintf("%s<=%d", opts.Name, opts.MaxConcurrentRequests),
	}, nil
}

// ReportMetrics updates all limiter's gauge metrics to match the current state.
//
// Must be called periodically (at least once per every metrics flush).
func (l *Limiter) ReportMetrics(ctx context.Context) {
	concurrencyCurGauge.Set(ctx, atomic.LoadInt64(&l.concurrency), l.opts.Name)
	concurrencyMaxGauge.Set(ctx, l.opts.MaxConcurrentRequests, l.opts.Name)
}

// CheckRequest should be called before processing a request.
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
		if cur >= l.opts.MaxConcurrentRequests && !l.opts.AdvisoryMode {
			return nil, l.reject(ctx, ri, "max concurrency")
		}
		if !atomic.CompareAndSwapInt64(&l.concurrency, cur, cur+1) {
			continue // race, try again
		}
		// Now that we have definitely grabbed the execution slot, report the
		// advisory rejection message. Doing it sooner may result in duplications.
		if cur >= l.opts.MaxConcurrentRequests && l.opts.AdvisoryMode {
			_ = l.reject(ctx, ri, "max concurrency") // actually ignore the error
		}
		return func() { atomic.AddInt64(&l.concurrency, -1) }, nil
	}
}

// reject is called when the request is rejected (either for real or in
// advisory mode).
//
// It updates metrics and logs and returns an annotated ErrLimitReached error.
func (l *Limiter) reject(ctx context.Context, ri *RequestInfo, reason string) error {
	rejectedCounter.Add(ctx, 1, l.opts.Name, ri.CallLabel, ri.PeerLabel, reason)
	if l.opts.AdvisoryMode {
		logging.Warningf(ctx, "limiter %q in advisory mode: the request hit the %s limit", l.titleForLog, reason)
	} else {
		logging.Errorf(ctx, "limiter %q: the request hit the %s limit", l.titleForLog, reason)
	}
	return errors.Fmt("limiter %q: %s limit: %w", l.titleForLog, reason, ErrLimitReached)
}
