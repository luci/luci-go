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

import { expect } from 'chai';
import { destroy } from 'mobx-state-tree';
import sinon from 'sinon';

import { FakeServiceWorker } from '../../libs/test_utils/fake_service_worker';
import { ServiceWorkerState } from './service_worker_state';

describe('ServiceWorkerState', () => {
  it('should sync properties correctly', () => {
    const state = ServiceWorkerState.create();
    after(() => destroy(state));

    const stubbedServiceWorker = sinon.stub(new FakeServiceWorker('installing', 'sw.js'));

    state.init(stubbedServiceWorker);
    expect(state.serviceWorker).to.eq(stubbedServiceWorker);
    expect(state.state).to.eq('installing');
    expect(stubbedServiceWorker.addEventListener.callCount).to.eq(1);

    const [event, listener] = stubbedServiceWorker.addEventListener.getCall(0).args;
    expect(event).to.eq('statechange');

    stubbedServiceWorker.state = 'installed';
    if ('handleEvent' in listener) {
      listener.handleEvent(new Event('statechange'));
    } else {
      listener(new Event('statechange'));
    }

    expect(state.state).to.eq('installed');
  });
});
