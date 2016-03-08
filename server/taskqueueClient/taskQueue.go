// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package taskqueueClient

import (
	"fmt"
	"time"

	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry"
	"golang.org/x/net/context"
	"google.golang.org/api/taskqueue/v1beta2"
)

// taskQueue is an abstraction wrapper around the task queue API.
type taskQueue interface {
	lease(c context.Context, tasks int, d time.Duration) ([]*Task, error)
	deleteTask(c context.Context, task string) error
	updateLease(c context.Context, task string, d time.Duration) (*Task, error)
}

type taskQueueImpl struct {
	*taskqueue.Service

	project string
	queue   string
	tag     string
}

func (tq *taskQueueImpl) lease(c context.Context, tasks int, d time.Duration) ([]*Task, error) {
	call := tq.Tasks.Lease(tq.project, tq.queue, int64(tasks), dtos(d)).Context(c)
	if tq.tag != "" {
		call.Tag(tq.tag)
	}

	var apiTasks *taskqueue.Tasks
	err := retry.Retry(c, retry.Default, func() (err error) {
		apiTasks, err = call.Do()
		return
	}, func(err error, delay time.Duration) {
		log.Fields{
			log.ErrorKey: err,
			"delay":      delay,
			"tasks":      tasks,
			"lease":      d,
		}.Warningf(c, "Error leasing tasks. Retrying...")
	})
	if err != nil {
		return nil, err
	}

	// If we got nil apiTasks, operate as if we got no tasks.
	if apiTasks == nil {
		return nil, nil
	}

	// Sanity check: if our task queue returned more tasks than we requested,
	// ignore those past our threshold. This shouldn't happen, but since we're
	// calling a remote API it's better to accommodate the chance.
	if len(apiTasks.Items) > tasks {
		apiTasks.Items = apiTasks.Items[:tasks]
	}

	// Translate to our Task type.
	t := make([]*Task, len(apiTasks.Items))
	for i, at := range apiTasks.Items {
		var err error
		t[i], err = buildTask(at)
		if err != nil {
			return nil, fmt.Errorf("failed to build Task #%d: %v", i, err)
		}
	}
	return t, nil
}

func (tq *taskQueueImpl) deleteTask(c context.Context, task string) error {
	call := tq.Tasks.Delete(tq.project, tq.queue, task).Context(c)
	return retry.Retry(c, retry.Default, func() error {
		return call.Do()
	}, func(err error, delay time.Duration) {
		log.Fields{
			log.ErrorKey: err,
			"delay":      delay,
			"task":       task,
		}.Warningf(c, "Error deleting task. Retrying...")
	})
}

func (tq *taskQueueImpl) updateLease(c context.Context, task string, d time.Duration) (*Task, error) {
	call := tq.Tasks.Patch(tq.project, tq.queue, task, dtos(d), &taskqueue.Task{}).Context(c)

	var apiTask *taskqueue.Task
	err := retry.Retry(c, retry.Default, func() (err error) {
		apiTask, err = call.Do()
		return
	}, func(err error, delay time.Duration) {
		log.Fields{
			log.ErrorKey: err,
			"task":       task,
			"delay":      delay,
			"lease":      d,
		}.Warningf(c, "Error updating task lease. Retrying...")
	})
	if err != nil {
		return nil, err
	}

	t, err := buildTask(apiTask)
	if err != nil {
		return nil, fmt.Errorf("failed to build Task: %v", err)
	}
	return t, nil
}

// dtos converts a duration to a seconds count.
func dtos(d time.Duration) int64 {
	return int64(d.Seconds())
}
