// Copyright 2025 The LUCI Authors.
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

// Package app contains pub/sub handlers.
package app

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/pubsub"

	"go.chromium.org/luci/analysis/internal/ingestion/join"
)

var (
	rootInvocationsFinalizedCounter = metric.NewCounter(
		"analysis/ingestion/pubsub/root_invocations_finalized",
		"The number of finalized root invocations received by LUCI Analysis from PubSub.",
		nil,
		// The LUCI Project.
		field.String("project"),
		// "success", "transient-failure", "permanent-failure" or "ignored".
		field.String("status"))
)

type handleRootInvocationMethod func(ctx context.Context, notification *rdbpb.RootInvocationFinalizedNotification) (processed bool, err error)

// RootInvocationFinalizedHandler accepts and processes ResultDB Root Invocation
// Finalized Pub/Sub messages.
type RootInvocationFinalizedHandler struct {
	// The method to use to handle the deserialized root invocation finalized
	// notification. Used to allow the handler to be replaced for testing.
	joinRootInvocation handleRootInvocationMethod
}

// NewRootInvocationFinalizedHandler initialises a new
// RootInvocationFinalizedHandler.
func NewRootInvocationFinalizedHandler() *RootInvocationFinalizedHandler {
	return &RootInvocationFinalizedHandler{
		joinRootInvocation: join.JoinRootInvocation,
	}
}

// Handle processes the root invocation finalized pubsub message.
func (h *RootInvocationFinalizedHandler) Handle(ctx context.Context, message pubsub.Message, notification *rdbpb.RootInvocationFinalizedNotification) error {
	status := "unknown"
	project := "unknown"
	defer func() {
		// Closure for late binding.
		rootInvocationsFinalizedCounter.Add(ctx, 1, project, status)
	}()

	rootInvocation := notification.GetRootInvocation()
	if rootInvocation == nil {
		return errors.New("root invocation must be specified")
	}

	// Schedule artifact ingestion task only for Android root invocation.
	project, _ = realms.Split(rootInvocation.Realm)
	if project == "android" {
		// TODO: Schedule the artifact ingest task for the root invocation.
	}

	// Handle join invocation.
	processed, err := h.joinRootInvocation(ctx, notification)
	if err == nil && !processed {
		err = pubsub.Ignore.Apply(errors.New("ignoring root invocation finalized notification"))
	}
	status = errStatus(err)
	return err
}
