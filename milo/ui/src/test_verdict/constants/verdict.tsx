// Copyright 2024 The LUCI Authors.
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

import {
  Cancel,
  CheckCircle,
  RemoveCircle,
  Report,
  Warning,
} from '@mui/icons-material';

import { TestVariantStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

export const VERDICT_STATUS_COLOR_MAP = Object.freeze({
  [TestVariantStatus.EXONERATED]: 'var(--exonerated-color)',
  [TestVariantStatus.EXPECTED]: 'var(--success-color)',
  [TestVariantStatus.FLAKY]: 'var(--warning-color)',
  [TestVariantStatus.UNEXPECTED]: 'var(--failure-color)',
  [TestVariantStatus.UNEXPECTEDLY_SKIPPED]: 'var(--critical-failure-color)',
});

export const VERDICT_STATUS_ICON_MAP = Object.freeze({
  [TestVariantStatus.EXONERATED]: (
    <RemoveCircle sx={{ color: 'var(--exonerated-color)' }} />
  ),
  [TestVariantStatus.EXPECTED]: (
    <CheckCircle sx={{ color: 'var(--success-color)' }} />
  ),
  [TestVariantStatus.FLAKY]: <Warning sx={{ color: 'var(--warning-color)' }} />,
  [TestVariantStatus.UNEXPECTED]: (
    <Cancel sx={{ color: 'var(--failure-color)' }} />
  ),
  [TestVariantStatus.UNEXPECTEDLY_SKIPPED]: (
    <Report sx={{ color: 'var(--critical-failure-color)' }} />
  ),
});
