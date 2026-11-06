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

import {
  getHealthStatus,
  HEALTHY_THRESHOLD_PCT,
  WARNING_THRESHOLD_PCT,
} from './health_status_utils';

describe('health_status_utils', () => {
  describe('constants', () => {
    it('defines expected threshold constants', () => {
      expect(HEALTHY_THRESHOLD_PCT).toBe(95);
      expect(WARNING_THRESHOLD_PCT).toBe(80);
    });
  });

  describe('getHealthStatus', () => {
    it('returns Healthy status for values >= 95%', () => {
      expect(getHealthStatus(95)).toEqual({
        status: 'Healthy',
        color: 'success',
      });
      expect(getHealthStatus(95.1)).toEqual({
        status: 'Healthy',
        color: 'success',
      });
      expect(getHealthStatus(100)).toEqual({
        status: 'Healthy',
        color: 'success',
      });
      expect(getHealthStatus(120)).toEqual({
        status: 'Healthy',
        color: 'success',
      });
    });

    it('returns Warning status for values in [80%, 95%)', () => {
      expect(getHealthStatus(94.9)).toEqual({
        status: 'Warning',
        color: 'warning',
      });
      expect(getHealthStatus(90)).toEqual({
        status: 'Warning',
        color: 'warning',
      });
      expect(getHealthStatus(80)).toEqual({
        status: 'Warning',
        color: 'warning',
      });
    });

    it('returns Critical status for values < 80%', () => {
      expect(getHealthStatus(79.9)).toEqual({
        status: 'Critical',
        color: 'error',
      });
      expect(getHealthStatus(50)).toEqual({
        status: 'Critical',
        color: 'error',
      });
      expect(getHealthStatus(0)).toEqual({
        status: 'Critical',
        color: 'error',
      });
    });
  });
});
