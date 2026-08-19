// Copyright 2026 The LUCI Authors.
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
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/pubsub"

	"go.chromium.org/luci/analysis/internal/workunits/exporter"
)

var (
	workUnitsNotificationCounter = metric.NewCounter(
		"analysis/ingestion/pubsub/work_units",
		"The number of work units notifications received by LUCI Analysis from PubSub.",
		nil,
		// "success", "transient-failure", "permanent-failure" or "ignored".
		field.String("status"))
)

// WorkUnitsPubSubHandler accepts and processes ResultDB Work Units PubSub
// messages.
type WorkUnitsPubSubHandler struct {
	exporter *exporter.Exporter
}

// NewWorkUnitsPubSubHandler initialises a new WorkUnitsPubSubHandler.
func NewWorkUnitsPubSubHandler(exporter *exporter.Exporter) *WorkUnitsPubSubHandler {
	return &WorkUnitsPubSubHandler{
		exporter: exporter,
	}
}

// Handle processes the work units pubsub message.
func (h *WorkUnitsPubSubHandler) Handle(ctx context.Context, message pubsub.Message, notification *rdbpb.WorkUnitsNotification) error {
	status := "unknown"
	defer func() {
		workUnitsNotificationCounter.Add(ctx, 1, status)
	}()

	realm := notification.GetRootInvocationMetadata().GetRealm()
	if !strings.Contains(realm, ":") {
		logging.Errorf(ctx, "invalid or missing realm %q in work units pubsub message payload", realm)
		err := pubsub.Ignore.Apply(errors.New("invalid or missing realm"))
		status = errStatus(err)
		return err
	}

	project, _ := realms.Split(realm)

	fields := logging.Fields{
		"Project":        project,
		"RootInvocation": notification.RootInvocationMetadata.RootInvocationId,
	}
	ctx = logging.SetFields(ctx, fields)

	exportOpts := exporter.Options{
		Project: project,
	}

	if err := h.exporter.Export(ctx, notification, exporter.WorkUnitTable, exportOpts); err != nil {
		status = errStatus(err)
		return err
	}

	status = "success"
	return nil
}
