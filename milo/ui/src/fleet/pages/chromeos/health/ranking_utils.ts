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

/**
 * Returns the MUI color key corresponding to a cohort's availability percentage.
 */
export const getHealthColor = (
  availabilityPct: number,
): 'success' | 'warning' | 'error' => {
  if (availabilityPct >= 85) return 'success';
  if (availabilityPct >= 70) return 'warning';
  return 'error';
};
