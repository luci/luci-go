// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package taskqueue

import (
	"github.com/luci/luci-go/common/errors"
)

type taskqueueImpl struct{ RawInterface }

func (t *taskqueueImpl) NewTask(path string) *Task {
	return &Task{Path: path}
}

func (t *taskqueueImpl) Add(task *Task, queueName string) error {
	return errors.SingleError(t.AddMulti([]*Task{task}, queueName))
}

func (t *taskqueueImpl) Delete(task *Task, queueName string) error {
	return errors.SingleError(t.DeleteMulti([]*Task{task}, queueName))
}

func (t *taskqueueImpl) AddMulti(tasks []*Task, queueName string) error {
	lme := errors.NewLazyMultiError(len(tasks))
	i := 0
	err := t.RawInterface.AddMulti(tasks, queueName, func(t *Task, err error) {
		if !lme.Assign(i, err) {
			*tasks[i] = *t
		}
		i++
	})
	if err == nil {
		err = lme.Get()
	}
	return err
}

func (t *taskqueueImpl) DeleteMulti(tasks []*Task, queueName string) error {
	lme := errors.NewLazyMultiError(len(tasks))
	i := 0
	err := t.RawInterface.DeleteMulti(tasks, queueName, func(err error) {
		lme.Assign(i, err)
		i++
	})
	if err == nil {
		err = lme.Get()
	}
	return err
}

func (t *taskqueueImpl) Purge(queueName string) error {
	return t.RawInterface.Purge(queueName)
}

func (t *taskqueueImpl) Stats(queueNames ...string) ([]Statistics, error) {
	ret := make([]Statistics, len(queueNames))
	lme := errors.NewLazyMultiError(len(queueNames))
	i := 0
	err := t.RawInterface.Stats(queueNames, func(s *Statistics, err error) {
		if !lme.Assign(i, err) {
			ret[i] = *s
		}
		i++
	})
	if err == nil {
		err = lme.Get()
	}
	return ret, err
}

func (t *taskqueueImpl) Raw() RawInterface {
	return t.RawInterface
}

func (t *taskqueueImpl) Testable() Testable {
	return t.RawInterface.Testable()
}

var _ Interface = (*taskqueueImpl)(nil)
