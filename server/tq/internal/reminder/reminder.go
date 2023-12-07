// Copyright 2020 The LUCI Authors.
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

// Package reminder holds Reminder to avoid circular dependencies.
package reminder

import (
	"time"

	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"cloud.google.com/go/pubsub/apiv1/pubsubpb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/server/tq/internal/tqpb"
)

// FreshUntilPrecision is precision of Reminder.FreshUntil, to which it is
// always truncated.
const FreshUntilPrecision = time.Millisecond

// Reminder reminds to enqueue a task.
//
// It is persisted transactionally with some other user logic to the database.
// Later, a task is actually scheduled and a reminder can be deleted
// non-transactionally.
//
// Its payload is represented either by a raw byte buffer (when the reminder
// is stored and loaded), or by a more complex Go value (when the reminder
// is manipulated by Dispatcher and Submitter). The Go value representation
// is described by Payload struct and it can be "attached" to the reminder
// via AttachReminder() or deserialized from the raw byte buffer via Payload().
type Reminder struct {
	// ID identifies a reminder.
	//
	// ID values are always in hex-encoded and are well distributed in keyspace.
	ID string

	// FreshUntil is the expected time by which the happy path should complete.
	//
	// If the sweeper encounters a Reminder before this time, the sweeper ignores
	// it to allow the happy path to complete.
	//
	// Truncated to FreshUntilPrecision.
	FreshUntil time.Time

	// RawPayload is a proto-serialized tqpb.Payload.
	//
	// It is what is actually stored in the database.
	RawPayload []byte

	// payload, if non-nil, is attached (or deserialized) payload.
	payload *Payload
}

// AttachPayload attaches the given payload to this reminder.
//
// It mutates `p` with reminder's ID, which should already be populated.
//
// Panics if `r` has a payload attached already.
func (r *Reminder) AttachPayload(p *Payload) error {
	if r.payload != nil {
		panic("the reminder has a payload attached already")
	}
	p.injectReminderID(r.ID)

	msg := &tqpb.Payload{
		TaskClass: p.TaskClass,
		Created:   timestamppb.New(p.Created),
	}

	switch {
	case p.CreateTaskRequest != nil:
		blob, err := proto.Marshal(p.CreateTaskRequest)
		if err != nil {
			return errors.Annotate(err, "failed to marshal CreateTaskRequest").Err()
		}
		msg.Payload = &tqpb.Payload_CreateTaskRequest{CreateTaskRequest: blob}
	case p.PublishRequest != nil:
		blob, err := proto.Marshal(p.PublishRequest)
		if err != nil {
			return errors.Annotate(err, "failed to marshal PublishRequest").Err()
		}
		msg.Payload = &tqpb.Payload_PublishRequest{PublishRequest: blob}
	default:
		panic("malformed payload")
	}

	raw, err := proto.Marshal(msg)
	if err != nil {
		return errors.Annotate(err, "failed to marshal Payload").Err()
	}

	r.RawPayload = raw
	r.payload = p
	return nil
}

// DropPayload returns a copy of the reminder without attached payload.
func (r *Reminder) DropPayload() *Reminder {
	return &Reminder{
		ID:         r.ID,
		FreshUntil: r.FreshUntil,
		RawPayload: r.RawPayload,
	}
}

// MustHavePayload returns an attached payload or panics if `r` doesn't have
// a payload attached.
//
// Does not attempt to deserialize RawPayload.
func (r *Reminder) MustHavePayload() *Payload {
	if r.payload == nil {
		panic("the reminder doesn't have a payload attached")
	}
	return r.payload
}

// Payload returns an attached payload, perhaps deserializing it first.
func (r *Reminder) Payload() (*Payload, error) {
	if r.payload != nil {
		return r.payload, nil
	}

	var msg tqpb.Payload
	if err := proto.Unmarshal(r.RawPayload, &msg); err != nil {
		return nil, errors.Annotate(err, "failed to unmarshal Payload").Err()
	}

	p := &Payload{
		TaskClass: msg.TaskClass,
		Created:   msg.Created.AsTime(),
	}

	switch blob := msg.Payload.(type) {
	case *tqpb.Payload_CreateTaskRequest:
		req := &taskspb.CreateTaskRequest{}
		if err := proto.Unmarshal(blob.CreateTaskRequest, req); err != nil {
			return nil, errors.Annotate(err, "failed to unmarshal CreateTaskRequest").Err()
		}
		p.CreateTaskRequest = req
	case *tqpb.Payload_PublishRequest:
		req := &pubsubpb.PublishRequest{}
		if err := proto.Unmarshal(blob.PublishRequest, req); err != nil {
			return nil, errors.Annotate(err, "failed to unmarshal PublishRequest").Err()
		}
		p.PublishRequest = req
	default:
		return nil, errors.New("unrecognized task payload kind")
	}

	r.payload = p
	return p, nil
}
