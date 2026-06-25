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

package analysis

import (
	"testing"

	"cloud.google.com/go/bigquery"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	configpb "go.chromium.org/luci/analysis/proto/config"
)

func TestWhereThresholdsMet(t *testing.T) {
	t.Parallel()
	ftt.Run(`whereThresholdsMet`, t, func(t *ftt.Test) {
		metric := metrics.HumanClsFailedPresubmit
		threshold := &configpb.MetricThreshold{
			OneDay:   proto.Int64(10),
			ThreeDay: proto.Int64(20),
			SevenDay: proto.Int64(30),
		}

		t.Run(`With suffix`, func(t *ftt.Test) {
			sql, params := whereThresholdsMet(metric, threshold, "_suffix")
			assert.Loosely(t, sql, should.Equal(
				"INT64(metrics_1d_residual.human_cls_failed_presubmit) >= @metric_human_cls_failed_presubmit_1d_suffix OR "+
					"INT64(metrics_3d_residual.human_cls_failed_presubmit) >= @metric_human_cls_failed_presubmit_3d_suffix OR "+
					"INT64(metrics_7d_residual.human_cls_failed_presubmit) >= @metric_human_cls_failed_presubmit_7d_suffix"))

			assert.Loosely(t, params, should.Match([]bigquery.QueryParameter{
				{
					Name:  "metric_human_cls_failed_presubmit_1d_suffix",
					Value: int64(10),
				},
				{
					Name:  "metric_human_cls_failed_presubmit_3d_suffix",
					Value: int64(20),
				},
				{
					Name:  "metric_human_cls_failed_presubmit_7d_suffix",
					Value: int64(30),
				},
			}))
		})

		t.Run(`Empty suffix`, func(t *ftt.Test) {
			sql, params := whereThresholdsMet(metric, threshold, "")
			assert.Loosely(t, sql, should.Equal(
				"INT64(metrics_1d_residual.human_cls_failed_presubmit) >= @metric_human_cls_failed_presubmit_1d OR "+
					"INT64(metrics_3d_residual.human_cls_failed_presubmit) >= @metric_human_cls_failed_presubmit_3d OR "+
					"INT64(metrics_7d_residual.human_cls_failed_presubmit) >= @metric_human_cls_failed_presubmit_7d"))

			assert.Loosely(t, params, should.Match([]bigquery.QueryParameter{
				{
					Name:  "metric_human_cls_failed_presubmit_1d",
					Value: int64(10),
				},
				{
					Name:  "metric_human_cls_failed_presubmit_3d",
					Value: int64(20),
				},
				{
					Name:  "metric_human_cls_failed_presubmit_7d",
					Value: int64(30),
				},
			}))
		})
	})
}
