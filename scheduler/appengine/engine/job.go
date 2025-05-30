// Copyright 2017 The LUCI Authors.
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

package engine

import (
	"bytes"
	"context"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/scheduler/appengine/catalog"
	"go.chromium.org/luci/scheduler/appengine/engine/cron"
	"go.chromium.org/luci/scheduler/appengine/engine/dsset"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/schedule"
)

// FinishedInvocationsHorizon defines how many invocations to keep in the
// Job's FinishedInvocations list.
//
// All entries there that are older than FinishedInvocationsHorizon will be
// evicted next time the list is updated.
const FinishedInvocationsHorizon = 10 * time.Minute

// Job stores the last known definition of a scheduler job, as well as its
// current state. Root entity, its kind is "Job".
type Job struct {
	_kind  string                `gae:"$kind,Job"`
	_extra datastore.PropertyMap `gae:"-,extra"`

	// cachedSchedule and cachedScheduleErr are used by ParseSchedule().
	cachedSchedule    *schedule.Schedule `gae:"-"`
	cachedScheduleErr error              `gae:"-"`

	// JobID is '<ProjectID>/<JobName>' string. JobName is unique with a project,
	// but not globally. JobID is unique globally.
	JobID string `gae:"$id"`

	// ProjectID exists for indexing. It matches <projectID> portion of JobID.
	ProjectID string

	// RealmID is a global realm name (i.e. "<ProjectID>:...") the job belongs to.
	RealmID string

	// Flavor describes what category of jobs this is, see the enum.
	Flavor catalog.JobFlavor `gae:",noindex"`

	// Enabled is false if the job was disabled or removed from config.
	//
	// Disabled jobs do not show up in UI at all (they are still kept in the
	// datastore though, for audit purposes).
	Enabled bool

	// Paused is true if no new invocations of the job should be started.
	//
	// Paused jobs ignore the cron scheduler and incoming triggers. Triggers are
	// completely skipped (not even enqueued). Pausing a job clears the pending
	// triggers set.
	Paused bool `gae:",noindex"`

	// PausedOrResumedWhen is when the job was paused or resumed.
	PausedOrResumedWhen time.Time `gae:",noindex"`

	// PausedOrResumedBy is who paused or resumed the job the last.
	PausedOrResumedBy identity.Identity `gae:",noindex"`

	// PausedOrResumedReason is the reason the job was paused or resumed.
	PausedOrResumedReason string `gae:",noindex"`

	// Revision is last seen job definition revision.
	Revision string `gae:",noindex"`

	// RevisionURL is URL to human readable page with config file at
	// an appropriate revision.
	RevisionURL string `gae:",noindex"`

	// Schedule is the job's schedule in regular cron expression format.
	Schedule string `gae:",noindex"`

	// Task is the job's payload in serialized form. Opaque from the point of view
	// of the engine. See Catalog.UnmarshalTask().
	Task []byte `gae:",noindex"`

	// TriggeredJobIDs is a list of jobIDs of jobs which this job triggers.
	// The list is sorted and without duplicates.
	TriggeredJobIDs []string `gae:",noindex"`

	// Cron holds the state of the cron state machine.
	Cron cron.State `gae:",noindex"`

	// TriggeringPolicyRaw is job's TriggeringPolicy proto in serialized form.
	//
	// It is taken from the job definition stored in the catalog. Used during
	// the triage.
	TriggeringPolicyRaw []byte `gae:",noindex"`

	// ActiveInvocations is ordered set of active invocation IDs.
	//
	// It contains IDs of pending, running or recently finished invocations,
	// the most recent at the end.
	ActiveInvocations []int64 `gae:",noindex"`

	// FinishedInvocationsRaw is a list of recently finished invocations, along
	// with the time they finished.
	//
	// It is serialized internal.FinishedInvocationList proto, see db.proto. We
	// store it this way to simplify adding more fields if necessary and to avoid
	// paying the cost of the deserialization if the caller is not interested.
	//
	// This list is used to achieve a perfectly consistent listing of all recent
	// invocations of a job.
	//
	// Entries older than FinishedInvocationsHorizon are evicted from this list
	// during triages. We assume that FinishedInvocationsHorizon is enough for
	// datastore indexes to catch up, so all recent invocations older than the
	// horizon can be fetched using a regular datastore query.
	FinishedInvocationsRaw []byte `gae:",noindex"`

	// LastTriage is a time when the last triage transaction was committed.
	LastTriage time.Time `gae:",noindex"`
}

// JobTriageLog contains information about the most recent triage.
//
// To avoid increasing the triage transaction size, and to allow logging triage
// transaction collisions, this entity is saved non-transactionally in a
// separate entity group on a best effort basis.
//
// It means it may occasionally be stale. To detect staleness we duplicate
// LastTriage timestamp here. If Job.LastTriage indicates the triage happened
// sufficiently log ago (by wall clock), but JobTriageLog.LastTriage is still
// old, then the log is stale (since JobTriageLog commit should have landed
// already). When this happens consistently we'll have to use real GAE logs to
// figure out what's wrong.
type JobTriageLog struct {
	_kind  string                `gae:"$kind,JobTriageLog"`
	_extra datastore.PropertyMap `gae:"-,extra"`

	// JobID is '<ProjectID>/<JobName>' string, matches corresponding Job.JobID.
	JobID string `gae:"$id"`
	// LastTriage is set to exact same value as corresponding Job.LastTriage.
	LastTriage time.Time `gae:",noindex"`
	// DebugLog is short free form text log with debug messages.
	DebugLog string `gae:",noindex"`

	// stale is populated by GetJobTriageLog if it thinks the log is stale.
	stale bool `gae:"-"`
}

// Stale is true if the engine thinks the log is stale.
//
// It does it by comparing LastTriage to the job's LastTriage.
func (j *JobTriageLog) Stale() bool {
	return j.stale
}

// JobName returns name of this Job as defined its project's config.
//
// This is "<name>" part extracted from "<project>/<name>" job ID.
func (e *Job) JobName() string {
	chunks := strings.Split(e.JobID, "/")
	return chunks[1]
}

// EffectiveSchedule returns schedule string to use for the job, considering its
// Paused field.
//
// Paused jobs always use "triggered" schedule.
func (e *Job) EffectiveSchedule() string {
	if e.Paused {
		return "triggered"
	}
	return e.Schedule
}

// ParseSchedule returns *Schedule object, parsing e.Schedule field.
//
// If job is paused e.Schedule field is ignored and "triggered" schedule is
// returned instead.
func (e *Job) ParseSchedule() (*schedule.Schedule, error) {
	if e.cachedSchedule == nil && e.cachedScheduleErr == nil {
		hash := fnv.New64()
		hash.Write([]byte(e.JobID))
		seed := hash.Sum64()
		e.cachedSchedule, e.cachedScheduleErr = schedule.Parse(e.EffectiveSchedule(), seed)
		if e.cachedSchedule == nil && e.cachedScheduleErr == nil {
			panic("no schedule and no error")
		}
	}
	return e.cachedSchedule, e.cachedScheduleErr
}

// IsEqual returns true iff 'e' is equal to 'other'.
func (e *Job) IsEqual(other *Job) bool {
	return e == other || (e.JobID == other.JobID &&
		e.ProjectID == other.ProjectID &&
		e.RealmID == other.RealmID &&
		e.Flavor == other.Flavor &&
		e.Enabled == other.Enabled &&
		e.Paused == other.Paused &&
		e.PausedOrResumedWhen.Equal(other.PausedOrResumedWhen) &&
		e.PausedOrResumedBy == other.PausedOrResumedBy &&
		e.PausedOrResumedReason == other.PausedOrResumedReason &&
		e.Revision == other.Revision &&
		e.RevisionURL == other.RevisionURL &&
		e.Schedule == other.Schedule &&
		e.LastTriage.Equal(other.LastTriage) &&
		bytes.Equal(e.Task, other.Task) &&
		equalSortedLists(e.TriggeredJobIDs, other.TriggeredJobIDs) &&
		e.Cron.Equal(&other.Cron) &&
		bytes.Equal(e.TriggeringPolicyRaw, other.TriggeringPolicyRaw) &&
		equalInt64Lists(e.ActiveInvocations, other.ActiveInvocations) &&
		bytes.Equal(e.FinishedInvocationsRaw, other.FinishedInvocationsRaw))
}

// MatchesDefinition returns true if job definition in the entity matches the
// one specified by catalog.Definition struct.
func (e *Job) MatchesDefinition(def catalog.Definition) bool {
	return e.JobID == def.JobID &&
		e.RealmID == def.RealmID &&
		e.Flavor == def.Flavor &&
		e.Schedule == def.Schedule &&
		bytes.Equal(e.Task, def.Task) &&
		bytes.Equal(e.TriggeringPolicyRaw, def.TriggeringPolicy) &&
		equalSortedLists(e.TriggeredJobIDs, def.TriggeredJobIDs)
}

// CronTickTime returns time when the cron job is expected to start again.
//
// May return:
//
//	Zero time if the job is using relative schedule, or not a cron job at all.
//	schedule.DistantFuture if the job is paused.
func (e *Job) CronTickTime() time.Time {
	// Note: LastTick is "last scheduled tick", it is in the future.
	return e.Cron.LastTick.When
}

// recentlyFinishedSet is a set with IDs of all recently finished invocations.
//
// This is an accumulator of IDs to remove from ActiveInvocations list next
// time we run a triage for the corresponding Job.
//
// Invocation IDs are serialized with fmt.Sprintf("%d").
func recentlyFinishedSet(c context.Context, jobID string) *invocationIDSet {
	return &invocationIDSet{
		Set: dsset.Set{
			ID:              "finished:" + jobID,
			ShardCount:      8,
			TombstonesRoot:  datastore.KeyForObj(c, &Job{JobID: jobID}),
			TombstonesDelay: 30 * time.Minute,
		},
	}
}

// invocationIDSet is a dsset.Set that stores invocation IDs.
type invocationIDSet struct {
	dsset.Set
}

// Add adds a bunch of invocation IDs to the set.
func (s *invocationIDSet) Add(c context.Context, ids []int64) error {
	items := make([]dsset.Item, len(ids))
	for i, id := range ids {
		items[i].ID = fmt.Sprintf("%d", id)
	}
	return s.Set.Add(c, items)
}

// ItemToInvID takes a dsset.Item and returns invocation ID stored there or 0 if
// it's malformed.
func (s *invocationIDSet) ItemToInvID(i *dsset.Item) int64 {
	id, _ := strconv.ParseInt(i.ID, 10, 64)
	return id
}

// pendingTriggersSet is a set of not yet consumed triggers for the job.
//
// This is incoming triggers. They are processed in the triage procedure,
// resulting in new invocations.
func pendingTriggersSet(c context.Context, jobID string) *triggersSet {
	return &triggersSet{
		Set: dsset.Set{
			ID:              "triggers:" + jobID,
			ShardCount:      8,
			TombstonesRoot:  datastore.KeyForObj(c, &Job{JobID: jobID}),
			TombstonesDelay: 30 * time.Minute,
		},
	}
}

// triggersSet is a dsset.Set that stores internal.Trigger protos.
type triggersSet struct {
	dsset.Set
}

// Add adds triggers to the set.
func (s *triggersSet) Add(c context.Context, triggers []*internal.Trigger) error {
	items := make([]dsset.Item, 0, len(triggers))
	for _, t := range triggers {
		blob, err := proto.Marshal(t)
		if err != nil {
			return fmt.Errorf("failed to marshal proto - %s", err)
		}
		items = append(items, dsset.Item{
			ID:    t.Id,
			Value: blob,
		})
	}
	return s.Set.Add(c, items)
}

// Triggers returns all triggers in the set, in no particular order.
//
// Returns the original dsset listing (that can be used later with BeginPop or
// CleanupGarbage), as well as the actual deserialized set of triggers.
func (s *triggersSet) Triggers(c context.Context) (*dsset.Listing, []*internal.Trigger, error) {
	l, err := s.Set.List(c)
	if err != nil {
		return nil, nil, err
	}
	out := make([]*internal.Trigger, len(l.Items))
	for i, item := range l.Items {
		out[i] = &internal.Trigger{}
		if err := proto.Unmarshal(item.Value, out[i]); err != nil {
			return nil, nil, errors.Fmt("failed to unmarshal trigger: %w", err)
		}
		if out[i].Id != item.ID {
			return nil, nil, fmt.Errorf("trigger ID in the body (%q) doesn't match item ID %q", out[i].Id, item.ID)
		}
	}
	return l, out, nil
}
