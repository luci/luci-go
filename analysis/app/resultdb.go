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
	"regexp"
	"strconv"

	"go.chromium.org/luci/analysis/internal/ingestion/control"
	controlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/router"
	"google.golang.org/protobuf/encoding/protojson"
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

	// buildInvocationRE extracts the buildbucket build number from invocations
	// named after a buildbucket build ID.
	buildInvocationRE = regexp.MustCompile(`^build-([0-9]+)$`)
)

// InvocationFinalizedPubSubHandler accepts and processes ResultDB
// Invocation Finalized Pub/Sub messages.
func InvocationFinalizedPubSubHandler(ctx *router.Context) {
	status := "unknown"
	project := "unknown"
	defer func() {
		// Closure for late binding.
		invocationsFinalizedCounter.Add(ctx.Context, 1, project, status)
	}()
	project, processed, err := invocationFinalizedPubSubHandlerImpl(ctx.Context, ctx.Request)

	switch {
	case err != nil:
		errors.Log(ctx.Context, errors.Annotate(err, "handling invocation finalized pubsub event").Err())
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

func invocationFinalizedPubSubHandlerImpl(ctx context.Context, request *http.Request) (project string, processed bool, err error) {
	notification, err := extractInvocationFinalizedNotification(request)
	if err != nil {
		return "unknown", false, errors.Annotate(err, "failed to extract invocation finalized notification").Err()
	}

	project, _ = realms.Split(notification.Realm)
	processed, err = processInvocationFinalizedNotification(ctx, project, notification)
	if err != nil {
		return project, false, errors.Annotate(err, "processing notification").Err()
	}
	return project, processed, nil
}

func extractInvocationFinalizedNotification(r *http.Request) (*rdbpb.InvocationFinalizedNotification, error) {
	var msg pubsubMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		return nil, errors.Annotate(err, "decoding pubsub message").Err()
	}

	var run rdbpb.InvocationFinalizedNotification
	err := protojson.Unmarshal(msg.Message.Data, &run)
	if err != nil {
		return nil, errors.Annotate(err, "parsing pubsub message data").Err()
	}
	return &run, nil
}

func processInvocationFinalizedNotification(ctx context.Context, project string, notification *rdbpb.InvocationFinalizedNotification) (processed bool, err error) {
	id, err := pbutil.ParseInvocationName(notification.Invocation)
	if err != nil {
		return false, errors.Annotate(err, "parse invocation name").Err()
	}

	match := buildInvocationRE.FindStringSubmatch(id)
	if match == nil {
		// Invocations that are not of the form build-<BUILD ID> are ignored.
		return false, nil
	}
	bbBuildID, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return false, errors.Annotate(err, "parse build ID").Err()
	}

	// This proto has no fields for now. If we need to pass anything about the invocation,
	// we can add it in here in future.
	result := &controlpb.InvocationResult{}

	buildID := control.BuildID(bbHost, bbBuildID)
	if err := JoinInvocationResult(ctx, buildID, project, result); err != nil {
		return true, errors.Annotate(err, "joining invocation result").Err()
	}
	return true, nil
}
