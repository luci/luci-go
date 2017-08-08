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

package tasks

import (
	"fmt"
	"time"

	"go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/errors"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/retry/transient"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
)

// emptyArchiveDispatchTask is a singleton empty ArchiveDispatchTask.
//
// This is used because tq.Task are typed to their Payload, so every task needs
// a Payload. This is the default Payload value, and is overridden when the
// Payload needs to actually be populated for publishing.
var emptyArchiveDispatchTask = &logdog.ArchiveDispatchTask{}

// CreateArchivalTask adds a task to the task queue to initiate
// archival on an stream. The task will be delayed by "delay".
func CreateArchivalTask(c context.Context, id coordinator.HashID, tag logdog.ArchiveDispatchTask_Tag,
	delay time.Duration, params *coordinator.ArchivalParams) error {

	task := makeArchivalTask(c, id, tag)
	task.Payload = &logdog.ArchiveDispatchTask{
		Id:             string(id),
		Tag:            tag,
		SettleDelay:    google.NewDuration(params.SettleDelay),
		CompletePeriod: google.NewDuration(params.CompletePeriod),
	}
	task.Delay = delay

	// Add the task outside of a transaction, since it's a named task.
	//
	// This introduces two possibilities:
	// - During transaction retries, the task may be added multiple times. This
	//   is fine, since it will naturally deduplicate.
	// - If the overall transaction fails, the task may be added for a log stream
	//   that never exists. We handle this in the handler by warning (but not
	//   failing) on non-existent log streams.
	if err := taskDispatcher.AddTask(ds.WithoutTransaction(c), task); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"taskName":   task.Name(),
		}.Errorf(c, "Failed to add task to task queue.")
		return errors.Annotate(err, "failed to add task to task queue").Err()
	}

	log.Debugf(c, "Successfully created archival task: %q", task.Name())
	return nil
}

// DeleteArchiveStreamExpiredTask deletes stream EXPIRED task associated with
// id. If the task has already been deleted, this will do nothing.
func DeleteArchiveStreamExpiredTask(c context.Context, id coordinator.HashID) error {
	task := makeArchivalTask(c, id, logdog.ArchiveDispatchTask_EXPIRED)

	log.Debugf(c, "Deleting archival task: %q", task.Name())
	if err := taskDispatcher.DeleteTask(c, task); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"taskName":   task.Name(),
		}.Errorf(c, "Failed to delete expired archival task.")
		return errors.Annotate(err, "failed to delete expired archival task").Err()
	}

	log.Debugf(c, "Successfully removed EXPIRED archival task: %q", task.Name())
	return nil
}

// handleArchiveDispatchTask is a tq.Handler for an ArchiveDispatchTask.
//
// This task is associated with a log stream and some archival parameters. It
// will verify that the log stream hasn't had archival dispatched yet. If it
// has, the task will terminate without further operation.
//
// For streams that haven't been archived, this task will transactionally
// dispatch an archival task to the Archivist fleet and update the stream's
// status.
func handleArchiveDispatchTask(c context.Context, payload proto.Message, execCount int) error {
	adt, ok := payload.(*logdog.ArchiveDispatchTask)
	if !ok {
		return errors.Reason("unexpected message type %T", payload).Err()
	}

	log.Infof(c, "Handling archival for %q task (#%d) in namespace %q: %q",
		adt.Tag, execCount, info.GetNamespace(c), adt.Id)

	stream := coordinator.LogStream{ID: coordinator.HashID(adt.Id)}
	state := stream.State(c)

	// Check if we're already archived (non-transactional).
	if err := ds.Get(c, state); err != nil {
		if err == ds.ErrNoSuchEntity {
			log.Warningf(c, "Log stream no longer exists.")
			return nil
		}

		log.WithError(err).Errorf(c, "Failed to load archival log stream.")
		return errors.Annotate(err, "failed to load archival log stream").Tag(transient.Tag).Err()
	}
	if state.ArchivalState() != coordinator.NotArchived {
		log.Infof(c, "Log stream archival is already tasked.")
		return nil
	}

	// Get our archival publisher.
	svc := coordinator.GetServices(c)
	ap, err := svc.ArchivalPublisher(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to get archival publisher.")
		return errors.Annotate(err, "failed to get archival publisher").Tag(transient.Tag).Err()
	}
	defer func() {
		if err := ap.Close(); err != nil {
			log.WithError(err).Warningf(c, "Failed to close archival publisher.")
		}
	}()

	params := coordinator.ArchivalParams{
		RequestID:      info.RequestID(c),
		SettleDelay:    google.DurationFromProto(adt.SettleDelay),
		CompletePeriod: google.DurationFromProto(adt.CompletePeriod),
	}

	err = ds.RunInTransaction(c, func(c context.Context) error {
		// Check if we're already archived (transactional).
		if err := ds.Get(c, state); err != nil {
			if err == ds.ErrNoSuchEntity {
				log.Warningf(c, "(Transactional) Log stream no longer exists.")
				return nil
			}

			log.WithError(err).Errorf(c, "(Transactional) Failed to load archival log stream.")
			return errors.Annotate(err, "failed to load archival stream").Err()
		}
		if state.ArchivalState() != coordinator.NotArchived {
			log.Infof(c, "(Transactional) Log stream archival is already tasked.")
			return nil
		}

		if err = params.PublishTask(c, ap, state); err != nil {
			if err == coordinator.ErrArchiveTasked {
				log.Warningf(c, "Archival already tasked, skipping.")
				return nil
			}

			log.WithError(err).Errorf(c, "Failed to publish archival task.")
			return errors.Annotate(err, "failed to publish archival task").Err()
		}

		if err := ds.Put(c, state); err != nil {
			log.WithError(err).Errorf(c, "Failed to update datastore.")
			return errors.Annotate(err, "failed to update datastore").Err()
		}

		return nil
	}, nil)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to publish archival task.")
		return errors.Annotate(err, "failed to publish archival task").Tag(transient.Tag).Err()
	}

	log.Debugf(c, "Successfully published cleanup archival task.")
	return nil
}

func makeArchivalTask(c context.Context, id coordinator.HashID, tag logdog.ArchiveDispatchTask_Tag) *tq.Task {
	name := fmt.Sprintf("%s_%s", id, tag)
	return &tq.Task{
		Payload:          emptyArchiveDispatchTask,
		NamePrefix:       name,
		DeduplicationKey: info.GetNamespace(c),
		Title:            name,
	}
}
