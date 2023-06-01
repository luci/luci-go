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

import { beforeEach, expect, jest } from '@jest/globals';
import { destroy } from 'mobx-state-tree';

import { FakeServiceWorker } from '../../libs/test_utils/fake_service_worker';
import { FakeServiceWorkerRegistration } from '../../libs/test_utils/fake_service_worker_registration';
import {
  ServiceWorkerRegistrationState,
  ServiceWorkerRegistrationStateInstance,
} from './service_worker_registration_state';

describe('ServiceWorkerRegistrationState', () => {
  let fakeRegistration: FakeServiceWorkerRegistration;
  let state: ServiceWorkerRegistrationStateInstance;
  let addEventListenerStub: jest.SpiedFunction<
    typeof FakeServiceWorkerRegistration.prototype.addEventListener
  >;

  beforeEach(() => {
    fakeRegistration = new FakeServiceWorkerRegistration('/scope');
    addEventListenerStub = jest.spyOn(fakeRegistration, 'addEventListener');
    const removeEventListenerStub = jest.spyOn(
      fakeRegistration,
      'removeEventListener'
    );
    addEventListenerStub.mockImplementation(() => {});
    removeEventListenerStub.mockImplementation(() => {});
    state = ServiceWorkerRegistrationState.create();
  });
  afterEach(() => {
    destroy(state);
  });

  it('should sync properties correctly', () => {
    state.init(fakeRegistration);
    expect(state.waiting).toBeUndefined();
    expect(state.installing?.serviceWorker).toBeUndefined();
    expect(addEventListenerStub.mock.calls.length).toStrictEqual(1);

    const [event, listener] = addEventListenerStub.mock.calls[0];
    expect(event).toStrictEqual('updatefound');

    // Discovered a new installing service worker.
    const fakeSW = new FakeServiceWorker('installing', 'sw.js');
    jest.spyOn(fakeSW, 'addEventListener').mockImplementation(() => {});
    jest.spyOn(fakeSW, 'removeEventListener').mockImplementation(() => {});
    fakeRegistration.installing = fakeSW;
    if ('handleEvent' in listener) {
      listener.handleEvent(new Event('updatefound'));
    } else {
      listener(new Event('updatefound'));
    }

    expect(state.waiting).toBeUndefined();
    expect(state.installing?.serviceWorker).toStrictEqual(fakeSW);

    // The installing service worker becomes installed.
    fakeSW.state = 'installed';
    state.installing?._setState('installed');

    expect(state.waiting?.serviceWorker).toStrictEqual(fakeSW);
    expect(state.installing?.serviceWorker).toBeUndefined();

    // The installed service worker becomes activated.
    fakeSW.state = 'activated';
    state.waiting?._setState('activated');

    expect(state.waiting).toBeUndefined();
    expect(state.installing?.serviceWorker).toBeUndefined();
  });
});
