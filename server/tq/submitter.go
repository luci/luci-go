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

package tq

import (
	"context"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
	"google.golang.org/protobuf/proto"
)

// CloudTaskSubmitter implements Submitter on top of Cloud Tasks client.
type CloudTaskSubmitter struct {
	Client *cloudtasks.Client
}

// CreateTask creates a task, returning a gRPC status.
func (s *CloudTaskSubmitter) CreateTask(ctx context.Context, req *taskspb.CreateTaskRequest, _ proto.Message) error {
	_, err := s.Client.CreateTask(ctx, req)
	return err
}
