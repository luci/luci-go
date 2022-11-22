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
import { FakeServiceWorkerRegistration } from '../../libs/test_utils/fake_service_worker_registration';
import { ServiceWorkerRegistrationState } from './service_worker_registration_state';

describe('ServiceWorkerRegistrationState', () => {
  it('should sync properties correctly', () => {
    const state = ServiceWorkerRegistrationState.create();
    after(() => destroy(state));

    const stubbedRegistration = sinon.stub(new FakeServiceWorkerRegistration('/scope'));

    state.init(stubbedRegistration);
    expect(state.waiting).to.be.undefined;
    expect(state.installing?.serviceWorker).to.be.undefined;
    expect(stubbedRegistration.addEventListener.callCount).to.eq(1);

    const [event, listener] = stubbedRegistration.addEventListener.getCall(0).args;
    expect(event).to.eq('updatefound');

    // Discovered a new installing service worker.
    const stubbedSW = sinon.stub(new FakeServiceWorker('installing', 'sw.js'));
    stubbedRegistration.installing = stubbedSW;
    if ('handleEvent' in listener) {
      listener.handleEvent(new Event('updatefound'));
    } else {
      listener(new Event('updatefound'));
    }

    expect(state.waiting).to.be.undefined;
    expect(state.installing?.serviceWorker).to.eq(stubbedSW);

    // The installing service worker becomes installed.
    stubbedSW.state = 'installed';
    state.installing?._setState('installed');

    expect(state.waiting?.serviceWorker).to.eq(stubbedSW);
    expect(state.installing?.serviceWorker).to.be.undefined;

    // The installed service worker becomes activated.
    stubbedSW.state = 'activated';
    state.waiting?._setState('activated');

    expect(state.waiting).to.be.undefined;
    expect(state.installing?.serviceWorker).to.be.undefined;
  });
});
