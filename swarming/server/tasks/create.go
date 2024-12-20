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

package tasks

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/swarming/server/model"
)

// Creation contains information to create a new task.
type Creation struct {
	// RequestID is used to make new task request idempotent.
	RequestID string

	// Request is the TaskRequest entity representing the new task.
	Request *model.TaskRequest

	// SecretBytes is the SecretBytes entity.
	SecretBytes *model.SecretBytes
}

// Run creates and stores all the entities to create a new task.
//
// Returns a grpc error if there's an issue.
func (s *Creation) Run(ctx context.Context) (*model.TaskResultSummary, error) {
	if s.RequestID != "" {
		tri := &model.TaskRequestID{
			Key: model.TaskRequestIDKey(ctx, s.RequestID),
		}
		switch err := datastore.Get(ctx, tri); {
		case err == nil:
			trs, subErr := model.TaskResultSummaryFromID(ctx, tri.TaskID)
			if subErr != nil {
				logging.Errorf(ctx, "Error fetching the task recorded for request ID %s: %s", s.RequestID, subErr)
				return nil, status.Errorf(codes.Internal, "datastore error fetching the task")
			}
			return trs, nil
		case !errors.Is(err, datastore.ErrNoSuchEntity):
			return nil, status.Errorf(codes.Internal, "datastore error fetching the task")
		}
	}
	return nil, status.Errorf(codes.Unimplemented, "not implemented yet")
}
