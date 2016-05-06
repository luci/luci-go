// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mutations

import (
	"fmt"
	"time"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

// CreateArchiveTask is a tumble Mutation that registers a single hierarchy
// component.
type CreateArchiveTask struct {
	// Path is the path of the log stream to create an archive task for.
	Path types.StreamPath

	// Expiration is the time when an archive task should be forced regardless
	// of stream termination state.
	Expiration time.Time
}

var _ tumble.DelayedMutation = (*CreateArchiveTask)(nil)

// RollForward implements tumble.DelayedMutation.
func (m *CreateArchiveTask) RollForward(c context.Context) ([]tumble.Mutation, error) {
	c = log.SetField(c, "path", m.Path)

	svc := coordinator.GetServices(c)
	ap, err := svc.ArchivalPublisher(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to get archival publisher.")
		return nil, err
	}

	// Get the log stream.
	di := ds.Get(c)
	ls := m.logStream()
	if err := di.Get(ls); err != nil {
		if err == ds.ErrNoSuchEntity {
			log.Warningf(c, "Log stream no longer exists.")
			return nil, nil
		}

		log.WithError(err).Errorf(c, "Failed to load archival log stream.")
		return nil, err
	}

	params := coordinator.ArchivalParams{
		RequestID: info.Get(c).RequestID(),
	}
	if err = params.PublishTask(c, ap, ls); err != nil {
		if err == coordinator.ErrArchiveTasked {
			log.Warningf(c, "Archival already tasked, skipping.")
			return nil, nil
		}

		log.WithError(err).Errorf(c, "Failed to publish archival task.")
		return nil, err
	}

	if err := di.Put(ls); err != nil {
		log.WithError(err).Errorf(c, "Failed to update datastore.")
		return nil, err
	}

	log.Debugf(c, "Successfully published cleanup archival task.")
	return nil, nil
}

// Root implements tumble.DelayedMutation.
func (m *CreateArchiveTask) Root(c context.Context) *ds.Key {
	return ds.Get(c).KeyForObj(m.logStream())
}

// ProcessAfter implements tumble.DelayedMutation.
func (m *CreateArchiveTask) ProcessAfter() time.Time {
	return m.Expiration
}

// HighPriority implements tumble.DelayedMutation.
func (m *CreateArchiveTask) HighPriority() bool {
	return false
}

// TaskName returns the task's name, which is derived from its path.
func (m *CreateArchiveTask) TaskName(di ds.Interface) (*ds.Key, string) {
	ls := m.logStream()
	return di.KeyForObj(ls), fmt.Sprintf("archive-expired-%s", ls.HashID)
}

func (m *CreateArchiveTask) logStream() *coordinator.LogStream {
	return coordinator.LogStreamFromPath(m.Path)
}

func init() {
	tumble.Register((*CreateArchiveTask)(nil))
}
