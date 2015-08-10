// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package featureBreaker

import (
	"golang.org/x/net/context"

	tq "github.com/luci/gae/service/taskqueue"
)

type tqState struct {
	*state

	tq tq.RawInterface
}

var _ tq.RawInterface = (*tqState)(nil)

func (t *tqState) AddMulti(tasks []*tq.Task, queueName string, cb tq.RawTaskCB) error {
	return t.run(func() (err error) { return t.tq.AddMulti(tasks, queueName, cb) })
}

func (t *tqState) DeleteMulti(tasks []*tq.Task, queueName string, cb tq.RawCB) error {
	return t.run(func() error { return t.tq.DeleteMulti(tasks, queueName, cb) })
}

func (t *tqState) Purge(queueName string) error {
	return t.run(func() error { return t.tq.Purge(queueName) })
}

func (t *tqState) Stats(queueNames []string, cb tq.RawStatsCB) error {
	return t.run(func() error { return t.tq.Stats(queueNames, cb) })
}

func (t *tqState) Testable() tq.Testable {
	return t.tq.Testable()
}

// FilterTQ installs a counter TaskQueue filter in the context.
func FilterTQ(c context.Context, defaultError error) (context.Context, FeatureBreaker) {
	state := newState(defaultError)
	return tq.AddRawFilters(c, func(ic context.Context, tq tq.RawInterface) tq.RawInterface {
		return &tqState{state, tq}
	}), state
}
