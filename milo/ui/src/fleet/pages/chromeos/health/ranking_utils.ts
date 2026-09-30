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

const MS_IN_24_HOURS = 24 * 60 * 60 * 1000;

export interface TrendlinePointLike {
  readonly timestamp?: string | undefined;
  readonly value: number;
}

export interface DeltaCalculationResult {
  readonly availabilityPct: number;
  readonly prevAvailabilityPct: number;
  readonly healthDrop: number;
  readonly trend: 'up' | 'down' | 'flat';
}

/**
 * Calculates current availability and 24h health delta by comparing
 * the latest data point with the historical point closest to 24 hours prior.
 */
export const calculate24hDelta = (
  points: readonly TrendlinePointLike[],
): DeltaCalculationResult => {
  if (!points || points.length === 0) {
    return {
      availabilityPct: 0,
      prevAvailabilityPct: 0,
      healthDrop: 0,
      trend: 'flat',
    };
  }

  const latestPoint = points[points.length - 1];
  const availabilityPct = Math.min(
    100,
    Math.max(0, Math.round(latestPoint.value * 100)),
  );

  let prevPoint = latestPoint;
  if (latestPoint.timestamp) {
    const latestTime = new Date(latestPoint.timestamp).getTime();
    if (!isNaN(latestTime)) {
      const targetTime24hAgo = latestTime - MS_IN_24_HOURS;
      let closestDiff = Infinity;
      for (const p of points) {
        if (!p.timestamp) continue;
        const pTime = new Date(p.timestamp).getTime();
        if (isNaN(pTime)) continue;
        const diff = Math.abs(pTime - targetTime24hAgo);
        if (diff < closestDiff) {
          closestDiff = diff;
          prevPoint = p;
        }
      }
    }
  } else if (points.length >= 2) {
    prevPoint = points[0];
  }

  const prevAvailabilityPct = Math.min(
    100,
    Math.max(0, Math.round(prevPoint.value * 100)),
  );

  const healthDrop = Math.max(0, prevAvailabilityPct - availabilityPct);
  const trend: 'up' | 'down' | 'flat' =
    availabilityPct > prevAvailabilityPct
      ? 'up'
      : availabilityPct < prevAvailabilityPct
        ? 'down'
        : 'flat';

  return {
    availabilityPct,
    prevAvailabilityPct,
    healthDrop,
    trend,
  };
};

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
