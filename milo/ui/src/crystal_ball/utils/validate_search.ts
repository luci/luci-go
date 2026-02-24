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

import { SearchMeasurementsRequest } from '@/crystal_ball/types';

/**
 * Possible validation errors for a search measurements request.
 */
export interface ValidationErrors {
  metricKeys?: string;
}

/**
 * Validates a SearchMeasurementsRequest.
 * @param request - The request to validate.
 * @returns An object containing any validation errors.
 */
export function validateSearchRequest(
  request: Partial<SearchMeasurementsRequest>,
): ValidationErrors {
  const newErrors: ValidationErrors = {};
  if (!request.metricKeys || request.metricKeys.length === 0) {
    newErrors.metricKeys = 'At least one metric key is required.';
  }

  return newErrors;
}
