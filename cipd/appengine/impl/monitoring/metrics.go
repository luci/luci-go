// Copyright 2019 The LUCI Authors.
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

package monitoring

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/server/auth"
)

var bytesRequested = metric.NewCounter(
	"downloads/bytes",
	"Bytes requested for download by clients.",
	&types.MetricMetadata{Units: types.Bytes},
	field.String("client_name"),
	field.String("client_email"),
	field.String("download_source"),
)

// FileSize handles file size information. If client requests are being
// monitored it will be reported.
func FileSize(ctx context.Context, bytes uint64) {
	cfg, err := monitoringConfig(ctx)
	if err != nil {
		errors.Log(ctx, errors.Annotate(err, "failed to fetch monitoring config").Err())
		return
	}
	if cfg == nil {
		return
	}
	// Technically uint64 -> int64 may overflow but there are bigger problems than
	// a panic if a user requested a CIPD package larger than math.MaxInt64 bytes.
	bytesRequested.Add(ctx, int64(bytes), cfg.Label, string(auth.CurrentIdentity(ctx)), "GCS")
}
