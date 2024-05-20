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
	"encoding/json"
	"net/http"

	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/analysis/internal/ingestion/join"
	"go.chromium.org/luci/analysis/internal/ingestion/joinlegacy"
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
	handleInvocation       handleInvocationMethod
	handleInvocationLegacy handleInvocationMethod
}

// NewInvocationFinalizedHandler initialises a new InvocationFinalizedHandler.
func NewInvocationFinalizedHandler() *InvocationFinalizedHandler {
	return &InvocationFinalizedHandler{
		handleInvocation:       join.JoinInvocation,
		handleInvocationLegacy: joinlegacy.JoinInvocation,
	}
}

// Handle processes a ResultDB Invocation Finalized Pub/Sub message.
func (h *InvocationFinalizedHandler) Handle(ctx *router.Context) {
	status := "unknown"
	project := "unknown"
	defer func() {
		// Closure for late binding.
		invocationsFinalizedCounter.Add(ctx.Request.Context(), 1, project, status)
	}()
	project, processed, err := h.handleImpl(ctx.Request.Context(), ctx.Request)

	switch {
	case err != nil:
		errors.Log(ctx.Request.Context(), errors.Annotate(err, "handling invocation finalized pubsub event").Err())
		status = processErr(ctx, err)
		return
	case !processed:
		status = "ignored"
		// Use subtly different "success" response codes to surface in
		// standard GAE logs whether an ingestion was ignored or not,
		// while still acknowledging the pub/sub.
		// See https://cloud.google.com/pubsub/docs/push#receiving_messages.
		ctx.Writer.WriteHeader(http.StatusNoContent) // 204
	default:
		status = "success"
		ctx.Writer.WriteHeader(http.StatusOK)
	}
}

func (h *InvocationFinalizedHandler) handleImpl(ctx context.Context, request *http.Request) (project string, processed bool, err error) {
	notification, err := extractNotification(request)
	if err != nil {
		return "unknown", false, errors.Annotate(err, "failed to extract invocation finalized notification").Err()
	}

	project, _ = realms.Split(notification.Realm)
	processed, err = h.handleInvocationLegacy(ctx, notification)
	if err != nil {
		return project, false, errors.Annotate(err, "processing notification").Err()
	}
	// Run the WIP handler after the legacy handler,
	// so that failure of the WIP handler doesn't affect the existing handler.
	if _, handleInvocationErr := h.handleInvocation(ctx, notification); handleInvocationErr != nil {
		// Don't return the error so that it doesn't interrupt the existing handler.
		logging.Errorf(ctx, "failed to handle invocation with the new handler: %v", handleInvocationErr)
	}
	return project, processed, nil
}

func extractNotification(r *http.Request) (*rdbpb.InvocationFinalizedNotification, error) {
	var msg pubsubMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		return nil, errors.Annotate(err, "decoding pubsub message").Err()
	}

	var run rdbpb.InvocationFinalizedNotification
	unmarshalOpts := protojson.UnmarshalOptions{DiscardUnknown: true}
	err := unmarshalOpts.Unmarshal(msg.Message.Data, &run)
	if err != nil {
		return nil, errors.Annotate(err, "parsing pubsub message data").Err()
	}
	return &run, nil
}
