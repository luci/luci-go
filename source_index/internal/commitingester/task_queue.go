// Copyright 2024 The LUCI Authors.
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

package commitingester

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/source_index/internal/commitingester/taskspb"

	// Add support for Spanner transactions in TQ.
	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

const (
	commitIngestionTaskClass = "commit-ingestion"
	commitIngestionQueue     = "commit-ingestion"
)

var commitIngestion = tq.RegisterTaskClass(tq.TaskClass{
	ID:        commitIngestionTaskClass,
	Prototype: &taskspb.IngestCommits{},
	Queue:     commitIngestionQueue,
	Kind:      tq.Transactional,
})

// RegisterTaskQueueHandlers registers the task queue handlers for Source
// Index's commit ingestion.
func RegisterTaskQueueHandlers(srv *server.Server) error {
	commitIngestion.AttachHandler(handleCommitIngestion)

	return nil
}

// scheduleCommitIngestion enqueues a task to ingest commits from a Gitiles
// repository starting from the specified commitish.
func scheduleCommitIngestion(ctx context.Context, task *taskspb.IngestCommits) {
	tq.MustAddTask(ctx, &tq.Task{
		Title:   fmt.Sprintf("%s-%s-%s-page-%d", task.Host, task.Repository, task.Commitish, task.TaskIndex),
		Payload: task,
	})
}

func handleCommitIngestion(ctx context.Context, payload proto.Message) error {
	task := payload.(*taskspb.IngestCommits)

	ctx = logging.SetField(ctx, "host", task.Host)
	ctx = logging.SetField(ctx, "repository", task.Repository)
	ctx = logging.SetField(ctx, "commitish", task.Commitish)
	ctx = logging.SetField(ctx, "task_index", task.TaskIndex)

	logging.Infof(ctx, "received commit ingestion task with page token: %q", task.PageToken)

	// TODO(b/356027716): ingest the commits.

	return nil
}
