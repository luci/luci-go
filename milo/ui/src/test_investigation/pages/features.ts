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

import { createFeatureFlag } from '@/common/feature_flags/context';

export const NEW_TEST_INVESTIGATION_PAGE_FLAG = createFeatureFlag({
  namespace: 'test-investigation-ui',
  name: 'auto-redirect-to-new-ui',
  percentage: 0,
  description:
    'Redirects all accesses of the legacy build page test results tab to the new test investigation UI.',
  trackingBug: 'b:422604579',
});
