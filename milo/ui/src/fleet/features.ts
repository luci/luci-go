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

import { createFeatureFlag } from '@/common/feature_flags';

export const enableModernInventoryCards = createFeatureFlag({
  description: 'Enable modern ChromeOS inventory visual cards view',
  namespace: 'fleet-console',
  name: 'modern-inventory-cards',
  percentage: 0,
  trackingBug: '388907865',
});

export const enableAndroidUtilizationMetrics = createFeatureFlag({
  description:
    'Displays average Android device utilization metrics, columns, and filters.',
  namespace: 'fleet-console',
  name: 'android-utilization-metrics',
  percentage: 0,
  trackingBug: '525089400',
});
