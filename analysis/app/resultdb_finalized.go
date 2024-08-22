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
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/pubsub"
	"go.chromium.org/luci/server/router"

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

func (h *InvocationFinalizedHandler) Handle(ctx context.Context, message pubsub.Message) error {
	status := "unknown"
	project := "unknown"
	defer func() {
		// Closure for late binding.
		invocationsFinalizedCounter.Add(ctx, 1, project, status)
	}()

	var err error
	project, processed, err := h.handleImpl(ctx, message)
	if err == nil && !processed {
		err = pubsub.Ignore.Apply(errors.Reason("ignoring invocation finalized notification").Err())
	}
	status = errStatus(err)
	return err
}

func (h *InvocationFinalizedHandler) handleImpl(ctx context.Context, message pubsub.Message) (project string, processed bool, err error) {
	notification, err := extractNotification(message)
	if err != nil {
		return "unknown", false, errors.Annotate(err, "extract invocation finalized notification").Err()
	}

	project, _ = realms.Split(notification.Realm)
	processed, err = h.handleInvocation(ctx, notification)
	if err != nil {
		return project, false, errors.Annotate(err, "processing notification").Err()
	}
	return project, processed, nil
}

func extractNotification(message pubsub.Message) (*rdbpb.InvocationFinalizedNotification, error) {
	var run rdbpb.InvocationFinalizedNotification
	unmarshalOpts := protojson.UnmarshalOptions{DiscardUnknown: true}
	err := unmarshalOpts.Unmarshal(message.Data, &run)
	if err != nil {
		return nil, errors.Annotate(err, "parsing pubsub message data").Err()
	}
	return &run, nil
}

// HandleLegacy processes a ResultDB Invocation Finalized Pub/Sub message.
func (h *InvocationFinalizedHandler) HandleLegacy(ctx *router.Context) {
	status := "unknown"
	project := "unknown"
	defer func() {
		// Closure for late binding.
		invocationsFinalizedCounter.Add(ctx.Request.Context(), 1, project, status)
	}()
	project, processed, err := h.handleImplLegacy(ctx.Request.Context(), ctx.Request)

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

func (h *InvocationFinalizedHandler) handleImplLegacy(ctx context.Context, request *http.Request) (project string, processed bool, err error) {
	notification, err := extractNotificationLegacy(request)
	if err != nil {
		return "unknown", false, errors.Annotate(err, "failed to extract invocation finalized notification").Err()
	}

	project, _ = realms.Split(notification.Realm)
	processed, err = h.handleInvocation(ctx, notification)
	if err != nil {
		return project, false, errors.Annotate(err, "processing notification").Err()
	}
	return project, processed, nil
}

func extractNotificationLegacy(r *http.Request) (*rdbpb.InvocationFinalizedNotification, error) {
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
