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

import './step_entry';

import { aTimeout, fixture, html } from '@open-wc/testing-helpers';
import { css, LitElement } from 'lit';
import { customElement } from 'lit/decorators.js';

import { BuildbucketStatus } from '@/common/services/buildbucket';
import { Store } from '@/common/store';
import { StepExt } from '@/common/store/build_state';
import { provider } from '@/generic_libs/tools/lit_context';
import {
  IntersectionNotifier,
  provideNotifier,
} from '@/generic_libs/tools/observer_element';

@customElement('milo-bp-step-entry-test-notifier-provider')
@provider
class NotifierProviderElement extends LitElement {
  @provideNotifier()
  notifier = new IntersectionNotifier({ root: this });

  protected render() {
    return html`<slot></slot>`;
  }

  static styles = css`
    :host {
      display: block;
      height: 100px;
      overflow-y: auto;
    }
  `;
}

describe('StepEntry', () => {
  test('can render a step without start time', async () => {
    const step = new StepExt({
      step: {
        name: 'stepname',
        status: BuildbucketStatus.Scheduled,
        startTime: undefined,
      },
      listNumber: '1.',
      selfName: 'stepname',
      depth: 0,
      index: 0,
    });
    await fixture<NotifierProviderElement>(html`
      <milo-bp-step-entry-test-notifier-provider>
        <milo-bp-step-entry
          .store=${Store.create()}
          .step=${step}
        ></milo-bp-step-entry>
      </milo-bp-step-entry-test-notifier-provider>
    `);
    await aTimeout(10);
  });
});
