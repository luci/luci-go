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

	"go.chromium.org/luci/common/errors"
)

// Submitter is used by Dispatcher to submit Cloud Tasks.
//
// It lives in the context, so that it can be mocked in tests. In production
// contexts (setup when using the tq server module), the submitter is
// initialized to be CloudTaskSubmitter. Tests will need to provide their own
// submitter (usually via TestingContext).
type Submitter interface {
	// CreateTask creates a task, returning a gRPC status.
	//
	// AlreadyExists status indicates the task with request name already exists.
	// Other statuses are handled using their usual semantics.
	//
	// `msg` is the original task payload. It is non-nil only when CreateTask
	// is called on a "happy path" (i.e. not from a sweeper). This is primarily
	// useful to capture task payloads in tests. Production implementations of
	// Submitter should generally ignore it.
	//
	// Will be called from multiple goroutines at once.
	CreateTask(ctx context.Context, req *taskspb.CreateTaskRequest, msg proto.Message) error
}

// CloudTaskSubmitter implements Submitter on top of Cloud Tasks client.
type CloudTaskSubmitter struct {
	Client *cloudtasks.Client
}

// CreateTask creates a task, returning a gRPC status.
func (s *CloudTaskSubmitter) CreateTask(ctx context.Context, req *taskspb.CreateTaskRequest, _ proto.Message) error {
	_, err := s.Client.CreateTask(ctx, req)
	return err
}

var submitterCtxKey = "go.chromium.org/luci/server/tq.Submitter"

// UseSubmitter puts an arbitrary submitter in the context.
//
// It will be used by Dispatcher's AddTask to submit Cloud Tasks.
func UseSubmitter(ctx context.Context, s Submitter) context.Context {
	return context.WithValue(ctx, &submitterCtxKey, s)
}

// currentSubmitter returns the Submitter in the context or an error.
func currentSubmitter(ctx context.Context) (Submitter, error) {
	sub, _ := ctx.Value(&submitterCtxKey).(Submitter)
	if sub == nil {
		return nil, errors.New("not a valid TQ context, no Submitter available")
	}
	return sub, nil
}
