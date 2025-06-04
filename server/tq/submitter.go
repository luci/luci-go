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
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	pubsub "cloud.google.com/go/pubsub/apiv1"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcmon"

	"go.chromium.org/luci/server/tq/internal/reminder"
)

// Submitter is used by Dispatcher to submit tasks.
//
// It lives in the context, so that it can be mocked in tests. In production
// contexts (setup when using the tq server module), the submitter is
// initialized to be CloudSubmitter. Tests will need to provide their own
// submitter (usually via TestingContext).
//
// Note that currently Submitter can only be implemented by structs in server/tq
// package, since its signature depends on an internal reminder.Payload type.
type Submitter interface {
	// Submit submits a task, returning a gRPC status.
	//
	// AlreadyExists status indicates the task with request name already exists.
	// Other statuses are handled using their usual semantics.
	//
	// Will be called from multiple goroutines at once.
	Submit(ctx context.Context, p *reminder.Payload) error
}

// CloudSubmitter implements Submitter on top of Google Cloud APIs.
type CloudSubmitter struct {
	tasks  *cloudtasks.Client
	pubsub *pubsub.PublisherClient
}

// NewCloudSubmitter creates a new submitter.
func NewCloudSubmitter(ctx context.Context, creds credentials.PerRPCCredentials) (*CloudSubmitter, error) {
	// gRPC options used for both Cloud Tasks and PubSub clients. Copy-pasted from
	// cloud.google.com/go/pubsub initialization.
	opts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time: 5 * time.Minute,
		})),
		option.WithGRPCDialOption(grpc.WithStatsHandler(&grpcmon.ClientRPCStatsMonitor{})),
		option.WithGRPCDialOption(grpc.WithPerRPCCredentials(creds)),
	}

	tasks, err := cloudtasks.NewClient(ctx, opts...)
	if err != nil {
		return nil, errors.Fmt("failed to initialize Cloud Tasks client: %w", err)
	}

	pubsub, err := pubsub.NewPublisherClient(ctx, opts...)
	if err != nil {
		tasks.Close()
		return nil, errors.Fmt("failed to initialize Cloud PubSub client: %w", err)
	}

	return &CloudSubmitter{tasks: tasks, pubsub: pubsub}, nil
}

// Close closes the submitter.
func (s *CloudSubmitter) Close() {
	s.tasks.Close()
	s.pubsub.Close()
}

// Submit creates a task, returning a gRPC status.
func (s *CloudSubmitter) Submit(ctx context.Context, p *reminder.Payload) (err error) {
	switch {
	case p.CreateTaskRequest != nil:
		_, err = s.tasks.CreateTask(ctx, p.CreateTaskRequest)
	case p.PublishRequest != nil:
		_, err = s.pubsub.Publish(ctx, p.PublishRequest)
	default:
		err = status.Errorf(codes.Internal, "unrecognized payload kind")
	}
	return
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
