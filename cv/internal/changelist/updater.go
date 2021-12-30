// Copyright 2021 The LUCI Authors.
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

package changelist

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/common"
)

const (
	// BatchCLUpdateTaskClass is the Task Class ID of the BatchUpdateCLTask,
	// which is enqueued only during a transaction.
	BatchUpdateCLTaskClass = "batch-update-cl"
	// CLUpdateTaskClass is the Task Class ID of the UpdateCLTask.
	UpdateCLTaskClass = "update-cl"
)

// UpdaterBackend abstracts out fetching CL details from code review backend.
type UpdaterBackend interface {
	// Kind identifies the backend.
	//
	// It's also the first part of the CL's ExternalID, e.g. "gerrit".
	// Must not contain a slash.
	Kind() string

	// LookupApplicableConfig returns the latest ApplicableConfig for the previously
	// saved CL.
	//
	// See CL.ApplicableConfig field doc for more details. Roughly, it finds which
	// LUCI projects are configured to watch this CL.
	//
	// Updater calls LookupApplicableConfig() before Fetch() in order to avoid
	// the unnecessary Fetch() call entirely, e.g. if the CL is up to date or if
	// the CL is definitely not watched by a specific LUCI project.
	//
	// Returns non-nil ApplicableConfig normally.
	// Returns nil ApplicableConfig if the previously saved CL state isn't
	// sufficient to confidently determine the ApplicableConfig.
	LookupApplicableConfig(ctx context.Context, saved *CL) (*ApplicableConfig, error)

	// Fetch fetches the CL in the context of a given project.
	//
	// If a given cl.ID is 0, it means the CL entity doesn't exist in Datastore.
	// The cl.ExternalID is always set.
	//
	// UpdatedHint, if not zero time, is the backend-originating timestamp of the
	// most recent CL update time. It's sourced by CV by e.g. polling or PubSub
	// subscription. It is useful to detect and work around backend's eventual
	// consistency.
	Fetch(ctx context.Context, cl *CL, luciProject string, updatedHint time.Time) (UpdateFields, error)

	// TQErrorSpec allows customizing logging and error TQ-specific handling.
	//
	// For example, Gerrit backend may wish to retry out of quota errors without
	// logging detailed stacktrace.
	TQErrorSpec() common.TQIfy
}

type UpdateFields struct {
	// TODO(tandrii): implement.
}

// Updater knows how to update CLs from relevant backend (e.g. Gerrit),
// notifying other CV parts as needed.
type Updater struct {
	tqd     *tq.Dispatcher
	mutator *Mutator

	rwmutex  sync.RWMutex // guards `backends`
	backends map[string]UpdaterBackend
}

// NewUpdater creates a new Updater.
//
// Starts without backends, but they ought to be added via RegisterBackend().
func NewUpdater(tqd *tq.Dispatcher, m *Mutator) *Updater {
	u := &Updater{
		tqd:      tqd,
		mutator:  m,
		backends: make(map[string]UpdaterBackend, 1),
	}
	tqd.RegisterTaskClass(tq.TaskClass{
		ID:           BatchUpdateCLTaskClass,
		Prototype:    &BatchUpdateCLTask{},
		Queue:        "update-cl",
		Quiet:        true,
		QuietOnError: true,
		Kind:         tq.Transactional,
		Handler: func(ctx context.Context, payload proto.Message) error {
			t := payload.(*BatchUpdateCLTask)
			err := u.handleBatch(ctx, t)
			return common.TQifyError(ctx, err)
		},
	})
	tqd.RegisterTaskClass(tq.TaskClass{
		ID:           UpdateCLTaskClass,
		Prototype:    &UpdateCLTask{},
		Queue:        "update-cl",
		Quiet:        true,
		QuietOnError: true,
		Kind:         tq.FollowsContext,
		Handler: func(ctx context.Context, payload proto.Message) error {
			t := payload.(*UpdateCLTask)
			// NOTE: unlike other TQ handlers code in CV, the common.TQifyError is
			// done inside the handler to allow per-backend definition of which errors
			// are retriable.
			return u.handleCL(ctx, t)
		},
	})
	return u
}

// RegisterBackend registers a backend.
//
// Panics if backend for the same kind is already registered.
func (u *Updater) RegisterBackend(b UpdaterBackend) {
	kind := b.Kind()
	if strings.ContainsRune(kind, '/') {
		panic(fmt.Errorf("backend %T of kind %q must not contain '/'", b, kind))
	}
	u.rwmutex.Lock()
	defer u.rwmutex.Unlock()
	if _, exists := u.backends[kind]; exists {
		panic(fmt.Errorf("backend %q is already registered", kind))
	}
	u.backends[kind] = b
}

// ScheduleBatch schedules update of several CLs.
func (u *Updater) ScheduleBatch(ctx context.Context, task *BatchUpdateCLTask) error {
	// TODO(tandrii): implement.
	return nil
}

// Schedule dispatches a TQ task. It should be used instead of the direct
// tq.AddTask to allow for consistent de-duplication.
func (u *Updater) Schedule(ctx context.Context, task *UpdateCLTask) error {
	return u.ScheduleDelayed(ctx, task, 0)
}

// ScheduleDelayed is same as Schedule but with a delay.
func (u *Updater) ScheduleDelayed(ctx context.Context, task *UpdateCLTask, delay time.Duration) error {
	// TODO(tandrii): implement.
	return nil
}

///////////////////////////////////////////////////////////////////////////////
// implementation details.

func (u *Updater) handleBatch(ctx context.Context, task *BatchUpdateCLTask) error {
	// TODO(tandrii): implement.
	return nil
}

func (u *Updater) handleCL(ctx context.Context, task *UpdateCLTask) error {
	ctx = logging.SetField(ctx, "project", task.GetLuciProject())
	// TODO(tandrii): implement.
	// TODO(tandrii): use backend-provided TQIfy spec.
	return common.TQifyError(ctx, nil)
}

func (u *Updater) backendFor(cl *CL) (UpdaterBackend, error) {
	kind, err := cl.ExternalID.kind()
	if err != nil {
		return nil, err
	}
	u.rwmutex.RLock()
	defer u.rwmutex.RUnlock()
	if b, exists := u.backends[kind]; exists {
		return b, nil
	}
	return nil, errors.Reason("%q backend is not supported", kind).Err()
}
