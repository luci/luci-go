// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package count

import (
	"golang.org/x/net/context"

	tq "github.com/luci/gae/service/taskqueue"
)

// TQCounter is the counter object for the TaskQueue service.
type TQCounter struct {
	AddMulti    Entry
	DeleteMulti Entry
	Purge       Entry
	Stats       Entry
}

type tqCounter struct {
	c *TQCounter

	tq tq.RawInterface
}

var _ tq.RawInterface = (*tqCounter)(nil)

func (t *tqCounter) AddMulti(tasks []*tq.Task, queueName string, cb tq.RawTaskCB) error {
	return t.c.AddMulti.up(t.tq.AddMulti(tasks, queueName, cb))
}

func (t *tqCounter) DeleteMulti(tasks []*tq.Task, queueName string, cb tq.RawCB) error {
	return t.c.DeleteMulti.up(t.tq.DeleteMulti(tasks, queueName, cb))
}

func (t *tqCounter) Purge(queueName string) error {
	return t.c.Purge.up(t.tq.Purge(queueName))
}

func (t *tqCounter) Stats(queueNames []string, cb tq.RawStatsCB) error {
	return t.c.Stats.up(t.tq.Stats(queueNames, cb))
}

func (t *tqCounter) Testable() tq.Testable {
	return t.tq.Testable()
}

// FilterTQ installs a counter TaskQueue filter in the context.
func FilterTQ(c context.Context) (context.Context, *TQCounter) {
	state := &TQCounter{}
	return tq.AddRawFilters(c, func(ic context.Context, tq tq.RawInterface) tq.RawInterface {
		return &tqCounter{state, tq}
	}), state
}
