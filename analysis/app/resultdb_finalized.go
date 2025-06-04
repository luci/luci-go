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
	invocationsFinalizedCounter = metric.NewCounter(
		"analysis/ingestion/pubsub/invocations_finalized",
		"The number of finalized invocations received by LUCI Analysis from PubSub.",
		nil,
		// The LUCI Project.
		field.String("project"),
		// "success", "transient-failure", "permanent-failure" or "ignored".
		field.String("status"))
)

type handleInvocationMethod func(ctx context.Context, notification *rdbpb.InvocationFinalizedNotification) (processed bool, err error)

// InvocationFinalizedHandler accepts and processes ResultDB
// Invocation Finalized Pub/Sub messages.
type InvocationFinalizedHandler struct {
	// The method to use to handle the deserialized invocation finalized
	// notification. Used to allow the handler to be replaced for testing.
	handleInvocation handleInvocationMethod
}

// NewInvocationFinalizedHandler initialises a new InvocationFinalizedHandler.
func NewInvocationFinalizedHandler() *InvocationFinalizedHandler {
	return &InvocationFinalizedHandler{
		handleInvocation: join.JoinInvocation,
	}
}

func (h *InvocationFinalizedHandler) Handle(ctx context.Context, message pubsub.Message, notification *rdbpb.InvocationFinalizedNotification) error {
	status := "unknown"
	project := "unknown"
	defer func() {
		// Closure for late binding.
		invocationsFinalizedCounter.Add(ctx, 1, project, status)
	}()

	project, _ = realms.Split(notification.Realm)
	processed, err := h.handleInvocation(ctx, notification)
	if err != nil {
		return errors.Fmt("processing notification: %w", err)
	}
	if err == nil && !processed {
		err = pubsub.Ignore.Apply(errors.New("ignoring invocation finalized notification"))
	}
	status = errStatus(err)
	return err
}
