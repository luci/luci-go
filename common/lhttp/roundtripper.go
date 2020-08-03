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

package lhttp

import (
	"net/http"

	"golang.org/x/sync/semaphore"
	"golang.org/x/time/rate"
)

// LimitRate returns an http.RoundTripper that limits the request rate.
func LimitRate(base http.RoundTripper, limiter *rate.Limiter) http.RoundTripper {
	return RoundTripper(func(r *http.Request) (*http.Response, error) {
		if err := limiter.Wait(r.Context()); err != nil {
			return nil, err
		}
		return base.RoundTrip(r)
	})
}

// LimitConcurrency returns an http.RoundTripper that limits request
// concurrency.
func LimitConcurrency(base http.RoundTripper, sem *semaphore.Weighted) http.RoundTripper {
	return RoundTripper(func(r *http.Request) (*http.Response, error) {
		if err := sem.Acquire(r.Context(), 1); err != nil {
			return nil, err
		}
		defer sem.Release(1)
		return base.RoundTrip(r)
	})
}

// RoundTripper is a function type that implements http.RoundTripper.
// It is a concise way of implementing a http.RoundTripper.
type RoundTripper func(r *http.Request) (*http.Response, error)

// RoundTrip implements http.RoundTripper.
func (rt RoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return rt(r)
}
