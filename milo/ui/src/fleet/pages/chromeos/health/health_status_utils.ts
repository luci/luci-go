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

export type HealthStatusBand = 'Healthy' | 'Warning' | 'Critical';
export type HealthStatusColor = 'success' | 'warning' | 'error';

/**
 * Maps availability / health percentage to SLA status bands:
 * - Healthy: >= 95% (success)
 * - Warning: 80% - 94% (warning)
 * - Critical: < 80% (error)
 */
export const getHealthStatus = (
  pct: number,
): { status: HealthStatusBand; color: HealthStatusColor } => {
  if (pct >= 95) {
    return { status: 'Healthy', color: 'success' };
  }
  if (pct >= 80) {
    return { status: 'Warning', color: 'warning' };
  }
  return { status: 'Critical', color: 'error' };
};
