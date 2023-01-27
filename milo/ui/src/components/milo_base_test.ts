// Copyright 2021 The LUCI Authors.
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

import { fixture, html } from '@open-wc/testing-helpers';
import { assert } from 'chai';
import { customElement } from 'lit/decorators.js';

import { MiloBaseElement } from './milo_base';

describe('milo_base', () => {
  it('should call disposers on disconnect in the correct order', async () => {
    let records: number[] = [];
    let count = 0;

    @customElement('milo-test-base')
    class TestBaseElement extends MiloBaseElement {
      connectedCallback() {
        super.connectedCallback();

        const disposerMsg1 = count++;
        this.addDisposer(() => records.push(disposerMsg1));

        const disposerMsg2 = count++;
        this.addDisposer(() => records.push(disposerMsg2));

        const disposerMsg3 = count++;
        this.addDisposer(() => records.push(disposerMsg3));
      }
    }

    const testBaseElement = await fixture<TestBaseElement>(html`<milo-test-base></milo-test-base>`);

    assert.deepEqual(records, []);
    testBaseElement.disconnectedCallback();
    assert.deepEqual(records, [2, 1, 0]);

    records = [];
    testBaseElement.connectedCallback();
    testBaseElement.disconnectedCallback();
    // Only disposers should've been cleared.
    assert.notDeepEqual(records, [5, 4, 3, 2, 1, 0]);
    assert.deepEqual(records, [5, 4, 3]);
  });
});
