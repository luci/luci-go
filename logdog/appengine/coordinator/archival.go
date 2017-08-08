// Copyright 2016 The LUCI Authors.
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

package coordinator

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"golang.org/x/net/context"
)

// ErrArchiveTasked is returned by ArchivalParams' PublishTask if the supplied
// LogStream indicates that it has already had an archival request dispatched.
var ErrArchiveTasked = errors.New("archival already tasked for this stream")

// ArchivalParams is the archival configuration.
type ArchivalParams struct {
	// RequestID is the unique request ID to use as a random base for the
	// archival key.
	RequestID string

	// SettleDelay is the amount of settle delay to attach to this request.
	SettleDelay time.Duration

	// CompletePeriod is the amount of time after the initial archival task is
	// executed when the task should fail if the stream is incomplete. After this
	// period has expired, the archival may complete successfully even if the
	// stream is missing log entries.
	CompletePeriod time.Duration
}

// PublishTask creates and dispatches a task queue task for the supplied
// LogStream. PublishTask is goroutine-safe.
//
// This should be run within a transaction on lst. On success, lst's state will
// be updated to reflect the archival tasking. This will NOT update lst's
// datastore entity; the caller must make sure to call Put within the same
// transaction for transactional safety.
//
// If the task is created successfully, this will return nil. If the LogStream
// already had a task dispatched, it will return ErrArchiveTasked.
func (p *ArchivalParams) PublishTask(c context.Context, ap ArchivalPublisher, lst *LogStreamState) error {
	if as := lst.ArchivalState(); as.Archived() || as == ArchiveTasked {
		// An archival task has already been dispatched for this log stream.
		return ErrArchiveTasked
	}

	id := lst.ID()
	msg := logdog.ArchiveTask{
		Project:      string(Project(c)),
		Id:           string(id),
		Key:          p.createArchivalKey(id, ap.NewPublishIndex()),
		DispatchedAt: google.NewTimestamp(clock.Now(c)),
	}
	if p.SettleDelay > 0 {
		msg.SettleDelay = google.NewDuration(p.SettleDelay)
	}
	if p.CompletePeriod > 0 {
		msg.CompletePeriod = google.NewDuration(p.CompletePeriod)
	}

	// Publish an archival request.
	if err := ap.Publish(c, &msg); err != nil {
		return err
	}

	// Update our LogStream's ArchiveState to reflect that an archival task has
	// been dispatched.
	lst.ArchivalKey = msg.Key
	return nil
}

// createArchivalKey returns a unique archival request key.
//
// The uniqueness is ensured by folding several components into a hash:
//	- The request ID, which is unique per HTTP request.
//	- The stream path.
//	- An atomically-incrementing key index, which is unique per ArchivalParams
//	  instance.
//
// The first two should be sufficient for a unique value, since a given request
// will only be handed to a single instance, and the atomic value is unique
// within the instance.
func (p *ArchivalParams) createArchivalKey(id HashID, pidx uint64) []byte {
	hash := sha256.New()
	if _, err := fmt.Fprintf(hash, "%s\x00%s\x00%d", p.RequestID, id, pidx); err != nil {
		panic(err)
	}
	return hash.Sum(nil)
}
