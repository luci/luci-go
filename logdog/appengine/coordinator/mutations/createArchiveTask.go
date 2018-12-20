// Copyright 2015 The LUCI Authors.
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

package mutations

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"

	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/appengine/coordinator/endpoints"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/common/clock"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/tumble"
)

// CreateArchiveTask is a tumble Mutation that registers an archive task.
//
// It is a named mutation.
type CreateArchiveTask struct {
	// ID is the hash ID of the LogStream whose archive task is being created.
	//
	// Note that the task will apply to the LogStreamState, not the stream
	// entity itself.
	ID coordinator.HashID

	// Key is the archive key of the previous archive attempt.
	// This is only provided if this archive task is a re-task.
	Key []byte

	// SettleDelay is the settle delay (see ArchivalParams).
	SettleDelay time.Duration
	// CompletePeriod is the complete period to use (see ArchivalParams).
	CompletePeriod time.Duration

	// Expiration is the delay applied to the archive task via ProcessAfter.
	Expiration time.Time
}

var _ tumble.DelayedMutation = (*CreateArchiveTask)(nil)

var (
	abandonedTasks = metric.NewCounter(
		"logdog/collector/archive/abandoned_tasks",
		("The number of abandoned archival tasks. (may be double-counted due " +
			"to transaction replay)"),
		nil,
		field.String("namespace"))
)

// RollForward implements tumble.DelayedMutation.
func (m *CreateArchiveTask) RollForward(c context.Context) ([]tumble.Mutation, error) {
	c = log.SetField(c, "id", m.ID)
	c = log.SetField(c, "key", m.Key)

	svc := endpoints.GetServices(c)
	ap, err := svc.ArchivalPublisher(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to get archival publisher.")
		return nil, err
	}
	defer func() {
		if err := ap.Close(); err != nil {
			log.WithError(err).Warningf(c, "Failed to close archival publisher.")
		}
	}()

	// Get the log stream.
	state := m.logStream().State(c)
	if err := ds.Get(c, state); err != nil {
		if err == ds.ErrNoSuchEntity {
			log.Warningf(c, "Log stream no longer exists.")
			return nil, nil
		}

		log.WithError(err).Errorf(c, "Failed to load archival log stream.")
		return nil, err
	}

	// If archived already, we're done.
	if state.ArchivalState().Archived() {
		return nil, nil
	}

	now := clock.Now(c)

	// If this was created more than three weeks ago, then abandon it. Issuing
	// a pubsub task won't help because the underlying data in bigtable is gone.
	threeWeeks := time.Hour * 24 * 7 * 3
	if now.After(state.Created.Add(threeWeeks)) {
		log.Warningf(c, "Abandoning old log stream: %q, %s -> %s (+3w %s)",
			m.ID, now, state.Created, state.Created.Add(threeWeeks))
		abandonedTasks.Add(c, 1, m.Root(c).Namespace())
		state.Updated = now
		state.ArchivedTime = now
		state.ArchivalKey = nil
		return nil, ds.Put(c, state)
	}

	params := coordinator.ArchivalParams{
		RequestID:      info.RequestID(c),
		PreviousKey:    m.Key,
		SettleDelay:    m.SettleDelay,
		CompletePeriod: m.CompletePeriod,
	}
	if err = params.PublishTask(c, ap, state); err != nil {
		if err == coordinator.ErrStreamArchived {
			log.Warningf(c, "Stream already archived, skipping.")
			return nil, nil
		}

		log.WithError(err).Errorf(c, "Failed to publish archival task.")
		return nil, err
	}

	if err := ds.Put(c, state); err != nil {
		log.WithError(err).Errorf(c, "Failed to update datastore.")
		return nil, err
	}

	log.Debugf(c, "Successfully published cleanup archival task.")
	return nil, nil
}

// Root implements tumble.DelayedMutation.
func (m *CreateArchiveTask) Root(c context.Context) *ds.Key {
	return ds.KeyForObj(c, m.logStream())
}

// ProcessAfter implements tumble.DelayedMutation.
func (m *CreateArchiveTask) ProcessAfter() time.Time { return m.Expiration }

// HighPriority implements tumble.DelayedMutation.
func (m *CreateArchiveTask) HighPriority() bool { return false }

// TaskName returns the task's name, which is derived from its log stream ID.
func (m *CreateArchiveTask) TaskName(c context.Context) string {
	return fmt.Sprintf("archive-expired-%s", m.ID)
}

// logStream returns the log stream associated with this task.
func (m *CreateArchiveTask) logStream() *coordinator.LogStream {
	return &coordinator.LogStream{
		ID: m.ID,
	}
}

func init() {
	tumble.Register((*CreateArchiveTask)(nil))
}
