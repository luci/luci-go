// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package schedule

import (
	"sync"
	"time"

	"github.com/gorhill/cronexpr"
)

// Schedule knows when to run the job (given current time and possibly current
// state of the job).
//
// See 'Parse' for a list of supported kinds of schedules.
type Schedule struct {
	asString string
	cronExpr *cronexpr.Expression
}

// Next tells when to run the job the next time.
func (s *Schedule) Next(now time.Time) time.Time {
	return s.cronExpr.Next(now)
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
//     schedule, not when the previous invocation finishes).
func Parse(expr string) (*Schedule, error) {
	gotIt, sched, err := cache.get(expr)
	if !gotIt {
		sched, err = doParse(expr)
		cache.put(expr, sched, err)
	}
	return sched, err
}

// doParse does the actual parsing (not using the cache).
func doParse(expr string) (*Schedule, error) {
	exp, err := cronexpr.Parse(expr)
	if err != nil {
		return nil, err
	}
	return &Schedule{
		asString: expr,
		cronExpr: exp,
	}, nil
}

////////////////////////////////////////////////////////////////////////////////

// scheduleCache implements dumb process-global cache for Schedules.
//
// Exists to save time on parsing schedules all the time when reading cron jobs
// from the datastore. Caches both valid and invalid schedules.
type scheduleCache struct {
	lock    sync.RWMutex
	entries map[string]interface{} // map of '*Schedule' or 'error' items
}

func (c *scheduleCache) init() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.entries = make(map[string]interface{}, 50)
}

func (c *scheduleCache) get(expr string) (bool, *Schedule, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	if val, ok := c.entries[expr]; ok {
		if sched, ok := val.(*Schedule); ok {
			return true, sched, nil
		}
		return true, nil, val.(error)
	}
	return false, nil, nil
}

func (c *scheduleCache) put(expr string, sched *Schedule, err error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if _, gotIt := c.entries[expr]; gotIt {
		return
	}
	// This cache called "dumb" for a reason.
	if len(c.entries) >= 500 {
		c.entries = make(map[string]interface{}, 50)
	}
	if sched != nil {
		c.entries[expr] = sched
	} else {
		c.entries[expr] = err
	}
}

//////////////

var cache scheduleCache

func init() {
	cache.init()
}
