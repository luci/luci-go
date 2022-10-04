// Copyright 2020 The LUCI Authors.
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

import { assert } from 'chai';
import { Duration } from 'luxon';

import { displayCompactDuration, displayDuration } from '../../src/libs/time_utils';

describe('Time Utils Tests', () => {
  describe('displayDuration', () => {
    it('should display correct duration in days and hours', async () => {
      const duration = Duration.fromISO('P3DT11H');
      assert.strictEqual(displayDuration(duration), '3 days 11 hours');
    });
    it('should display correct duration in hours and minutes', async () => {
      const duration = Duration.fromISO('PT1H11M');
      assert.strictEqual(displayDuration(duration), '1 hour 11 mins');
    });
    it('should display correct duration in minutes and seconds', async () => {
      const duration = Duration.fromISO('PT2M3S');
      assert.strictEqual(displayDuration(duration), '2 mins 3 secs');
    });
    it('should display correct duration in seconds', async () => {
      const duration = Duration.fromISO('PT15S');
      assert.strictEqual(displayDuration(duration), '15 secs');
    });
    it('should display correct duration in milliseconds', async () => {
      const duration = Duration.fromMillis(999);
      assert.strictEqual(displayDuration(duration), '999 ms');
    });
    it('should ignore milliseconds if duration > 1 second', async () => {
      const duration = Duration.fromMillis(1999);
      assert.strictEqual(displayDuration(duration), '1 sec');
    });
    it('should display zero duration correctly', async () => {
      const duration = Duration.fromMillis(0);
      assert.strictEqual(displayDuration(duration), '0 ms');
    });
  });

  describe('displayCompactDuration', () => {
    it("should display correct duration when it's null", async () => {
      assert.deepEqual(displayCompactDuration(null), ['N/A', '']);
    });
    it('should display correct duration in days', async () => {
      const duration = Duration.fromISO('P3DT11H');
      assert.deepEqual(displayCompactDuration(duration), ['3.5d', 'd']);
    });
    it('should display correct duration in hours', async () => {
      const duration = Duration.fromISO('PT1H11M');
      assert.deepEqual(displayCompactDuration(duration), ['1.2h', 'h']);
    });
    it('should display correct duration in minutes', async () => {
      const duration = Duration.fromISO('PT2M3S');
      assert.deepEqual(displayCompactDuration(duration), ['2.0m', 'm']);
    });
    it('should display correct duration in seconds', async () => {
      const duration = Duration.fromISO('PT15S');
      assert.deepEqual(displayCompactDuration(duration), ['15s', 's']);
    });
    it('should display correct duration in milliseconds', async () => {
      const duration = Duration.fromMillis(999);
      assert.deepEqual(displayCompactDuration(duration), ['999ms', 'ms']);
    });
    it("should not display the duration in ms if it's no less than 999.5ms", async () => {
      const duration = Duration.fromMillis(999.5);
      assert.deepEqual(displayCompactDuration(duration), ['1.0s', 's']);
    });
    it("should display the duration in ms if it's less than 999.5ms", async () => {
      const duration = Duration.fromMillis(999.4999);
      assert.deepEqual(displayCompactDuration(duration), ['999ms', 'ms']);
    });
  });
});
