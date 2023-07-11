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

package internal

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// BaseSampler constructs an object that decides how often to sample traces.
//
// The spec is a string in one of the forms:
//   - `X%` - to sample approximately X percent of requests.
//   - `Xqps` - to produce approximately X samples per second.
//
// Returns an error if the spec can't be parsed.
func BaseSampler(spec string) (trace.Sampler, error) {
	switch spec = strings.ToLower(spec); {
	case strings.HasSuffix(spec, "%"):
		percent, err := strconv.ParseFloat(strings.TrimSuffix(spec, "%"), 64)
		if err != nil {
			return nil, fmt.Errorf("not a float percent %q", spec)
		}
		// Note: TraceIDRatioBased takes care of <=0.0 && >=1.0 cases.
		return trace.TraceIDRatioBased(percent / 100.0), nil

	case strings.HasSuffix(spec, "qps"):
		qps, err := strconv.ParseFloat(strings.TrimSuffix(spec, "qps"), 64)
		if err != nil {
			return nil, fmt.Errorf("not a float QPS %q", spec)
		}
		if qps <= 0.0000001 {
			// Semantically the same, but slightly faster.
			return trace.TraceIDRatioBased(0), nil
		}
		return &qpsSampler{
			period: time.Duration(float64(time.Second) / qps),
			now:    time.Now,
			rnd:    rand.New(rand.NewSource(rand.Int63())),
		}, nil

	default:
		return nil, fmt.Errorf("unrecognized sampling spec string %q - should be either 'X%%' or 'Xqps'", spec)
	}
}

// GateSampler returns a sampler that calls the callback to decide if the span
// should be sampled.
//
// If the callback returns false, the span will not be sampled.
//
// If the callback returns true, the decision will be handed over to the given
// base sampler.
func GateSampler(base trace.Sampler, cb func(context.Context) bool) trace.Sampler {
	return &gateSampler{base, cb}
}

// qpsSampler asks to sample a trace approximately each 'period'.
//
// Adds some random jitter to desynchronize cycles running concurrently across
// many processes.
type qpsSampler struct {
	m      sync.RWMutex
	next   time.Time
	period time.Duration
	now    func() time.Time // for mocking time
	rnd    *rand.Rand       // for random jitter
}

func (s *qpsSampler) ShouldSample(p trace.SamplingParameters) trace.SamplingResult {
	now := s.now()

	s.m.RLock()
	sample := s.next.IsZero() || now.After(s.next)
	s.m.RUnlock()

	if sample {
		s.m.Lock()
		switch {
		case s.next.IsZero():
			// Start the cycle at some random offset.
			s.next = now.Add(s.randomDurationLocked(0, s.period))
		case now.After(s.next):
			// Add random jitter to the cycle length.
			jitter := s.period / 10
			s.next = now.Add(s.randomDurationLocked(s.period-jitter, s.period+jitter))
		}
		s.m.Unlock()
	}

	decision := trace.Drop
	if sample {
		decision = trace.RecordAndSample
	}
	return trace.SamplingResult{
		Decision:   decision,
		Tracestate: oteltrace.SpanContextFromContext(p.ParentContext).TraceState(),
	}
}

func (s *qpsSampler) Description() string {
	return fmt.Sprintf("qpsSampler{period:%s}", s.period)
}

func (s *qpsSampler) randomDurationLocked(min, max time.Duration) time.Duration {
	return min + time.Duration(s.rnd.Int63n(int64(max-min)))
}

// gateSampler is a sampler that calls the callback to decide if the span
// should be sampled.
type gateSampler struct {
	base trace.Sampler
	cb   func(context.Context) bool
}

func (s *gateSampler) ShouldSample(p trace.SamplingParameters) trace.SamplingResult {
	if !s.cb(p.ParentContext) {
		return trace.SamplingResult{
			Decision:   trace.Drop,
			Tracestate: oteltrace.SpanContextFromContext(p.ParentContext).TraceState(),
		}
	}
	return s.base.ShouldSample(p)
}

func (s *gateSampler) Description() string {
	return fmt.Sprintf("gateSampler{base:%s}", s.base.Description())
}
