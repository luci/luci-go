// Copyright 2025 The LUCI Authors.
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

export const RealmsTabFlag = createFeatureFlag({
  description: 'Flag for the realms permission tab in group details',
  namespace: 'authService',
  name: 'realmsPermissionsTab',
  percentage: 0,
  trackingBug: '350848587',
});

export const FullListingTabFlag = createFeatureFlag({
  description: 'Flag for the full listing tab in group details',
  namespace: 'authService',
  name: 'fullListingTab',
  percentage: 0,
  trackingBug: '350848877',
});

export const AncestorsTabFlag = createFeatureFlag({
  description: 'Flag for the ancestors tab in group details',
  namespace: 'authService',
  name: 'ancestorsFlag',
  percentage: 0,
  trackingBug: '350849077',
});
