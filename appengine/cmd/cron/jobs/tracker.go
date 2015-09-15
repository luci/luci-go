// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package jobs

import (
	"github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

// JobTracker kicks off invocations given their serialized descriptions and
// watches for them to finish (notifying Listener).
type JobTracker interface {
	// SetListener sets reference to an object that receives notifications about
	// finished invocations. Overrides previous listener. Must be called before
	// any other method.
	SetListener(l Listener)

	// Launch starts the actual job. If the method returns no errors, it means the
	// invocation has started and the tracker promises to call listener's
	// InvocationDone when invocation finishes. If it returns a non-transient
	// error, the invocation can't be started and retry won't help. If it returns
	// a transient error, the invocation may or may not have been started. Deal
	// with it.
	LaunchJob(c context.Context, jobID string, invocationID int64, work []byte) error
}

// Listener receives notification about finished invocations from JobTracker.
type Listener interface {
	InvocationDone(c context.Context, jobID string, invocationID int64) error
}

// NewJobTracker returns default implementation of JobTracker.
func NewJobTracker() JobTracker {
	return &jobTracker{}
}

type jobTracker struct {
	l Listener
}

func (t *jobTracker) SetListener(l Listener) {
	t.l = l
}

func (t *jobTracker) LaunchJob(c context.Context, jobID string, invocationID int64, work []byte) error {
	logging.Infof(c, "Doing work")
	return t.l.InvocationDone(c, jobID, invocationID)
}
