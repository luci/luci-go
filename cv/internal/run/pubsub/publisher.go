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

package pubsub

import (
	"context"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/server/tq"

	cvpb "go.chromium.org/luci/cv/api/v1"
	"go.chromium.org/luci/cv/internal/rpc/versioning"
	"go.chromium.org/luci/cv/internal/run"
)

const (
	v1RunEndedTopic     = "v1.run_ended"
	v1RunEndedTaskClass = "v1-publish-run-ended"
)

// Publisher publishes a message for various run events into Cloud PubSub
// topics.
type Publisher struct {
	tqd *tq.Dispatcher
}

// NewPublisher creates a new Publisher and registers TaskClasses for run
// events.
func NewPublisher(tqd *tq.Dispatcher) *Publisher {
	p := &Publisher{tqd}
	tqd.RegisterTaskClass(tq.TaskClass{
		ID:        v1RunEndedTaskClass,
		Topic:     v1RunEndedTopic,
		Prototype: &PublishRunEndedTask{},
		Kind:      tq.Transactional,
		Custom: func(ctx context.Context, m proto.Message) (*tq.CustomPayload, error) {
			t := m.(*PublishRunEndedTask)
			blob, err := (protojson.MarshalOptions{Indent: "\t"}).Marshal(&cvpb.PubSubRun{
				Id:       t.GetPublicId(),
				Status:   versioning.RunStatusV1(t.GetStatus()),
				Eversion: t.GetEversion(),
			})
			if err != nil {
				return nil, err
			}
			return &tq.CustomPayload{
				Meta: map[string]string{
					"status":       t.Status.String(),
					"luci_project": t.LuciProject,
				},
				Body: blob,
			}, nil
		},
	})
	return p
}

// RunEnded schedules a task to publish a RunEnded message.
func (s *Publisher) RunEnded(ctx context.Context, run *run.Run) error {
	return s.tqd.AddTask(ctx, &tq.Task{
		Payload: &PublishRunEndedTask{
			PublicId:    run.ID.PublicID(),
			LuciProject: run.ID.LUCIProject(),
			Status:      run.Status,
			Eversion:    int64(run.EVersion),
		},
	})
}
