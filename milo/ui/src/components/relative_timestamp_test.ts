// Copyright 2022 The LUCI Authors.
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

import { aTimeout, fixture, html } from '@open-wc/testing-helpers';
import { assert } from 'chai';
import { DateTime, Duration } from 'luxon';

import './relative_timestamp';
import { RelativeTimestampElement } from './relative_timestamp';

describe('relative_timestamp_test', () => {
  it('should display timestamp in the past correctly', async () => {
    const past = DateTime.now().minus(Duration.fromObject({ seconds: 20, millisecond: 500 }));

    const ele = await fixture<RelativeTimestampElement>(html`
      <milo-relative-timestamp .timestamp=${past}></milo-relative-timestamp>
    `);

    assert.equal(ele.relativeTimestamp, '20 secs ago');
  });

  it('should display timestamp in the future correctly', async () => {
    const past = DateTime.now().plus(Duration.fromObject({ seconds: 20, millisecond: 500 }));

    const ele = await fixture<RelativeTimestampElement>(html`
      <milo-relative-timestamp .timestamp=${past}></milo-relative-timestamp>
    `);
    assert.equal(ele.relativeTimestamp, 'in 20 secs');
  });

  it('should update timestamp correctly', async function () {
    this.timeout(10000);
    const past = DateTime.now().plus(Duration.fromObject({ seconds: 2, millisecond: 500 }));

    const ele = await fixture<RelativeTimestampElement>(html`
      <milo-relative-timestamp .timestamp=${past}></milo-relative-timestamp>
    `);
    assert.equal(ele.relativeTimestamp, 'in 2 secs');

    await aTimeout(5000);

    assert.equal(ele.relativeTimestamp, '2 secs ago');
  });
});
