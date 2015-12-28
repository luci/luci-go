// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package schedule

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/gorhill/cronexpr"
)

// Schedule knows when to run the job (given current time and possibly current
// state of the job).
//
// See 'Parse' for a list of supported kinds of schedules.
type Schedule struct {
	asString string
	randSeed uint64

	cronExpr *cronexpr.Expression // set for absolute schedules
	interval time.Duration        // set for relative schedules
}

// IsAbsolute is true for schedules that do not depend on a job state.
//
// Absolute schedules are basically static time tables specifying when to
// attempt to run a job.
//
// Non-absolute (aka relative) schedules may use job state transition times to
// make decisions.
//
// See comment for 'Parse' for some examples.
func (s *Schedule) IsAbsolute() bool {
	return s.cronExpr != nil
}

// Next tells when to run the job the next time.
//
// 'now' is current time. 'prev' is when previous invocation has finished (or
// zero time for first invocation).
func (s *Schedule) Next(now, prev time.Time) time.Time {
	// For an absolute schedule just look at the time table.
	if s.cronExpr != nil {
		return s.cronExpr.Next(now)
	}

	// Using relative schedule and this is a first invocation ever? Randomize
	// start time, so that a bunch of newly registered cron jobs do not start all
	// at once. Otherwise just wait for 'interval' seconds after previous
	// invocation.
	if prev.IsZero() {
		// Pass seed through math/rand to make small seeds (used by unit tests),
		// less special.
		rnd := rand.New(rand.NewSource(int64(s.randSeed))).Float64()
		return now.Add(time.Duration(float64(s.interval) * rnd))
	}
	next := prev.Add(s.interval)
	if next.Sub(now) < 0 {
		next = now
	}
	return next
}

// String serializes the schedule to a human readable string.
//
// It can be passed to Parse to get back the schedule.
func (s *Schedule) String() string {
	return s.asString
}

// Parse converts human readable definition of a schedule to *Schedule object.
//
// Supported kinds of schedules (illustrated by examples):
//   - "* 0 * * * *": standard cron-like expression. Cron engine will attempt
//     to start a job at specified moments in time (based on UTC clock). If when
//     triggering a job, previous invocation is still running, an overrun will
//     be recorded (and next attempt to start a job happens based on the
//     schedule, not when the previous invocation finishes). This is absolute
//     schedule (i.e. doesn't depend on job state).
//   - "with 10s interval": runs invocations in a loop, waiting 10s after
//     finishing invocation before starting a new one. This is relative
//     schedule. Overruns are not possible.
func Parse(expr string, randSeed uint64) (sched *Schedule, err error) {
	if strings.HasPrefix(expr, "with ") {
		sched, err = parseWithSchedule(expr, randSeed)
	} else {
		sched, err = parseCronSchedule(expr, randSeed)
	}
	if sched != nil {
		sched.asString = expr
		sched.randSeed = randSeed
	}
	return sched, err
}

// parseWithSchedule parses "with <interval> interval" schedule string.
func parseWithSchedule(expr string, randSeed uint64) (*Schedule, error) {
	tokens := strings.SplitN(expr, " ", 3)
	if len(tokens) != 3 || tokens[0] != "with" || tokens[2] != "interval" {
		return nil, errors.New("expecting format \"with <duration> interval\"")
	}
	interval, err := time.ParseDuration(tokens[1])
	if err != nil {
		return nil, fmt.Errorf("bad duration %q - %s", tokens[1], err)
	}
	if interval < 0 {
		return nil, fmt.Errorf("bad interval %q - it must be positive", tokens[1])
	}
	return &Schedule{interval: interval}, nil
}

// parseCronSchedule parses crontab-like schedule string.
func parseCronSchedule(expr string, randSeed uint64) (*Schedule, error) {
	exp, err := cronexpr.Parse(expr)
	if err != nil {
		return nil, err
	}
	return &Schedule{cronExpr: exp}, nil
}
