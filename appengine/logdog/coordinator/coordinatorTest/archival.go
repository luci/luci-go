// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinatorTest

import (
	"sort"
	"sync"

	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/logdog/types"
	"golang.org/x/net/context"
)

// ArchivalPublisher is a testing implementation of a
// coordinator.ArchivalPublisher. It records which archival tasks were
// scheduled and offers accessors to facilitate test assertions.
type ArchivalPublisher struct {
	sync.Mutex

	// Err, if not nil, is the error returned by Publish.
	Err error

	tasks []*logdog.ArchiveTask
}

var _ coordinator.ArchivalPublisher = (*ArchivalPublisher)(nil)

// Publish implements coordinator.ArchivalPublisher.
func (ap *ArchivalPublisher) Publish(c context.Context, at *logdog.ArchiveTask) error {
	ap.Lock()
	defer ap.Unlock()

	if err := ap.Err; err != nil {
		return err
	}

	ap.tasks = append(ap.tasks, at)
	return nil
}

// Tasks returns the dispatched archival tasks in the order in which they were
// dispatched.
func (ap *ArchivalPublisher) Tasks() []*logdog.ArchiveTask {
	ap.Lock()
	defer ap.Unlock()

	taskCopy := make([]*logdog.ArchiveTask, len(ap.tasks))
	for i, at := range ap.tasks {
		taskCopy[i] = at
	}
	return taskCopy
}

// StreamNames returns a sorted list of the "name" component of streams that
// have had archival tasks submitted.
func (ap *ArchivalPublisher) StreamNames() []string {
	tasks := ap.Tasks()

	taskStreams := make([]string, len(tasks))
	for i, at := range tasks {
		_, name := types.StreamPath(at.Path).Split()
		taskStreams[i] = string(name)
	}
	sort.Strings(taskStreams)
	return taskStreams
}
