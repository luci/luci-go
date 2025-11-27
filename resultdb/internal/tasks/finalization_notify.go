// Copyright 2022 The LUCI Authors.
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

// Package tasks contains task class definitions for ResultDB.
package tasks

import (
	"context"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	// Add support for Spanner transactions in TQ.
	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

const (
	v1NotifyFinalizedTaskClass           = "v1-publish-invocation-finalized"
	v1NotifyFinalizedTopic               = "v1.invocation_finalized"
	v1NotifyRootInvocationFinalizedTopic = "v1.root_invocation_finalized"
	V1NotifyTestResultsTopic             = "v1.test_results"

	// Pubsub message attributes
	androidBranchFilter = "primary_build_android_branch"
	androidTargetFilter = "primary_build_android_target"
	luciProjectFilter   = "luci_project"
	runnerFilter        = "tags_runner"
	stateFilter         = "state"
)

// NotifyFinalizedPublisher describes how to publish to cloud pub/sub
// notifications that an invocation has been finalized.
var NotifyFinalizedPublisher = tq.RegisterTaskClass(tq.TaskClass{
	ID:        "notify-finalized",
	Topic:     v1NotifyFinalizedTopic,
	Prototype: &taskspb.NotifyInvocationFinalized{},
	Kind:      tq.Transactional,
	Custom: func(ctx context.Context, m proto.Message) (*tq.CustomPayload, error) {
		// Custom serialisation handler needed to control
		// how the message is sent, as the backend is
		// Cloud Pub/Sub and not Cloud Tasks.
		t := m.(*taskspb.NotifyInvocationFinalized)
		notification := t.Message
		blob, err := (protojson.MarshalOptions{Indent: "\t"}).Marshal(notification)
		if err != nil {
			return nil, err
		}

		// Prepare attributes, which are can be used by subscribers to
		// filter the messages they receive.
		if err := realms.ValidateRealmName(notification.Realm, realms.GlobalScope); err != nil {
			return nil, err
		}
		project, _ := realms.Split(notification.Realm)
		attrs := map[string]string{
			"luci_project": project,
		}

		return &tq.CustomPayload{
			Meta: attrs,
			Body: blob,
		}, nil
	},
})

// NotifyRootInvocationFinalizedPublisher describes how to publish to cloud
// pub/sub notifications that a root invocation has been finalized.
var NotifyRootInvocationFinalizedPublisher = tq.RegisterTaskClass(tq.TaskClass{
	ID:        "notify-root-invocation-finalized",
	Topic:     v1NotifyRootInvocationFinalizedTopic,
	Prototype: &taskspb.NotifyRootInvocationFinalized{},
	Kind:      tq.Transactional,
	Custom: func(ctx context.Context, m proto.Message) (*tq.CustomPayload, error) {
		// Custom serialisation handler needed to control how the message is
		// sent, as the backend is Cloud Pub/Sub and not Cloud Tasks.
		t := m.(*taskspb.NotifyRootInvocationFinalized)
		notification := t.GetMessage()
		blob, err := (protojson.MarshalOptions{Indent: "\t"}).Marshal(notification)
		if err != nil {
			return nil, err
		}

		// Prepare attributes, which are can be used by subscribers to filter
		// the messages they receive.
		rootInvocation := notification.RootInvocation
		if err := realms.ValidateRealmName(rootInvocation.Realm, realms.GlobalScope); err != nil {
			return nil, err
		}

		return &tq.CustomPayload{
			Meta: attributes(rootInvocation),
			Body: blob,
		}, nil
	},
})

// NotifyTestResultsPublisher describes how to publish to cloud pub/sub
// notifications for test results.
var NotifyTestResultsPublisher = tq.RegisterTaskClass(tq.TaskClass{
	ID:        "notify-test-results",
	Topic:     V1NotifyTestResultsTopic,
	Prototype: &taskspb.PublishTestResults{},
	Kind:      tq.NonTransactional,
	Custom: func(ctx context.Context, m proto.Message) (*tq.CustomPayload, error) {
		// Custom serialisation handler needed to control how the message is
		// sent, as the backend is Cloud Pub/Sub and not Cloud Tasks.
		t := m.(*taskspb.PublishTestResults)
		notification := t.GetMessage()
		blob, err := proto.Marshal(notification)
		if err != nil {
			return nil, err
		}
		return &tq.CustomPayload{
			// Attributes can be used by subscribers to filter the messages they
			// receive.
			Meta: t.GetAttributes(),
			Body: blob,
		}, nil
	},
})

// attributes return the message attributes for subscribers to filter messages.
func attributes(rootInvocation *pb.RootInvocation) map[string]string {
	attrs := make(map[string]string)

	// Mandatory filters.
	project, _ := realms.Split(rootInvocation.Realm)
	attrs[luciProjectFilter] = project
	attrs[stateFilter] = rootInvocation.State.String()

	// Optional filters which are populated only when the info is available.
	// - Android filters
	if rootInvocation.PrimaryBuild != nil {
		if abd := rootInvocation.PrimaryBuild.GetAndroidBuild(); abd != nil {
			attrs[androidBranchFilter] = abd.Branch
			attrs[androidTargetFilter] = abd.BuildTarget
		}
	}

	properties := rootInvocation.GetProperties()
	if properties != nil {
		if runner := properties.GetFields()["runner"]; runner != nil {
			attrs[runnerFilter] = runner.GetStringValue()
		}
	}
	return attrs
}

// NotifyInvocationFinalized transactionally enqueues a task to publish that the
// given invocation has been finalized.
func NotifyInvocationFinalized(ctx context.Context, message *pb.InvocationFinalizedNotification) {
	tq.MustAddTask(ctx, &tq.Task{
		Payload: &taskspb.NotifyInvocationFinalized{Message: message},
	})
}

// NotifyRootInvocationFinalized transactionally enqueues a task to publish that
// the given root invocation has been finalized.
func NotifyRootInvocationFinalized(ctx context.Context, message *pb.RootInvocationFinalizedNotification) {
	tq.MustAddTask(ctx, &tq.Task{
		Payload: &taskspb.NotifyRootInvocationFinalized{Message: message},
	})
}

// NotifyTestResults transactionally enqueues a task to publish test results.
func NotifyTestResults(ctx context.Context, message *pb.TestResultsNotification, attrs map[string]string) {
	tq.MustAddTask(ctx, &tq.Task{
		Payload: &taskspb.PublishTestResults{Message: message, Attributes: attrs},
	})
}
