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

import { displayDuration } from '../../src/libs/time_utils';
import { Timestamp } from '../services/buildbucket';

describe('Time Utils Tests', () => {
  it('should display correct duration in days and hours', async () => {
    const beginTime: Timestamp = {seconds: 0, nanos: 0}
    const endTime: Timestamp = {seconds: 300000, nanos: 999999999}
    assert.strictEqual(displayDuration(beginTime, endTime), "3 days 11 hours");
  });
  it('should display correct duration in hours and minutes', async () => {
    const beginTime: Timestamp = {seconds: 0, nanos: 0}
    const endTime: Timestamp = {seconds: 4300, nanos: 999999999}
    assert.strictEqual(displayDuration(beginTime, endTime), "1 hour 11 mins");
  });
  it('should display correct duration in minutes and seconds', async () => {
    const beginTime: Timestamp = {seconds: 0, nanos: 0}
    const endTime: Timestamp = {seconds: 123, nanos: 999999999}
    assert.strictEqual(displayDuration(beginTime, endTime), "2 mins 3 secs");
  });
  it('should display correct duration in seconds', async () => {
    const beginTime: Timestamp = {seconds: 0, nanos: 0}
    const endTime: Timestamp = {seconds: 15, nanos: 999999999}
    assert.strictEqual(displayDuration(beginTime, endTime), "15 secs");
  });
  it('should display correct duration in milliseconds', async () => {
    const beginTime: Timestamp = {seconds: 0, nanos: 0}
    const endTime: Timestamp = {seconds: 0, nanos: 999999999}
    assert.strictEqual(displayDuration(beginTime, endTime), "999 ms");
  });
});