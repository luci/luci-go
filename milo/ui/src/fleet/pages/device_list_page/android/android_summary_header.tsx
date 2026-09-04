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

import { useFeatureFlag } from '@/common/feature_flags';
import { enableAndroidHealthMetrics } from '@/fleet/features';

import {
  AndroidHealthSummaryHeader,
  AndroidHealthSummaryHeaderProps,
} from './android_health_summary_header';
import {
  AndroidLegacySummaryHeader,
  AndroidLegacySummaryHeaderProps,
} from './android_legacy_summary_header';

export type AndroidSummaryHeaderProps = AndroidHealthSummaryHeaderProps &
  AndroidLegacySummaryHeaderProps;

export function AndroidSummaryHeader(props: AndroidSummaryHeaderProps) {
  const isAndroidHealthMetricsEnabled = useFeatureFlag(
    enableAndroidHealthMetrics,
  );

  if (isAndroidHealthMetricsEnabled) {
    return <AndroidHealthSummaryHeader {...props} />;
  }

  return <AndroidLegacySummaryHeader {...props} />;
}
