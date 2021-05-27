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

package bq

import (
	"context"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/server/tq"

	//"go.chromium.org/luci/common/errors"
	//"go.chromium.org/luci/common/logging"
	//"go.chromium.org/luci/common/retry/transient"

	cvbq "go.chromium.org/luci/cv/internal/bq"
	"go.chromium.org/luci/cv/internal/common"
)

// Sender sends BQ rows for Runs.
type Sender struct {
	tqd *tq.Dispatcher
	bqc cvbq.Client
}

// New creates a new Sender, registering it in the given TQ dispatcher.
func New(tqd *tq.Dispatcher, bqc cvbq.Client) *Sender {
	s := &Sender{tqd, bqc}
	tqd.RegisterTaskClass(tq.TaskClass{
		ID:        "send-run-row",
		Prototype: &SendRunRowTask{},
		Queue:     "send-run-row",
		Quiet:     true,
		Kind:      tq.NonTransactional,
		Handler: func(ctx context.Context, payload proto.Message) error {
			task := payload.(*SendRunRowTask)
			err := s.SendRunRow(ctx, task)
			return common.TQifyError(ctx, err)
		},
	})
	return s
}

// Schedule enqueues a task to send a row to BQ for a Run.
func (s *Sender) Schedule(ctx context.Context, t *SendRunRowTask) error {
	return tq.AddTask(ctx, &tq.Task{
		Payload: t,
	})
}

// SendRunRow sends a row.
func (s *Sender) SendRunRow(ctx context.Context, task *SendRunRowTask) error {
	return SendRun(ctx, s.bqc, common.RunID(task.RunId))
}
