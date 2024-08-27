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

package app

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	cvv1 "go.chromium.org/luci/cv/api/v1"
	"go.chromium.org/luci/server/pubsub"

	"go.chromium.org/luci/analysis/internal/ingestion/join"
)

var (
	cvRunCounter = metric.NewCounter(
		"analysis/ingestion/pubsub/cv_runs",
		"The number of CV runs received by LUCI Analysis from PubSub.",
		nil,
		// The LUCI Project.
		field.String("project"),
		// "success", "transient-failure", "permanent-failure" or "ignored".
		field.String("status"))
)

type handleCVRunMethod func(ctx context.Context, psRun *cvv1.PubSubRun) (project string, processed bool, err error)

// CVRunHandler accepts and processes CV Run Pub/Sub messages.
type CVRunHandler struct {
	// The method to use to handle the deserialized CV Run pub/sub message.
	// Used to allow the handler to be replaced for testing.
	handleCVRun handleCVRunMethod
}

// NewCVRunHandler initialises a new CVRunHandler.
func NewCVRunHandler() *CVRunHandler {
	return &CVRunHandler{
		handleCVRun: join.JoinCVRun,
	}
}

func (h *CVRunHandler) Handle(ctx context.Context, message pubsub.Message, cvMessage *cvv1.PubSubRun) error {
	status := "unknown"
	project := "unknown"
	defer func() {
		// Closure for late binding.
		cvRunCounter.Add(ctx, 1, project, status)
	}()

	var err error
	project, processed, err := h.handleCVRun(ctx, cvMessage)
	if err == nil && !processed {
		err = pubsub.Ignore.Apply(errors.Reason("ignoring CV run").Err())
	}
	status = errStatus(err)
	return err
}
