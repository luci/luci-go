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

import { destroy } from 'mobx-state-tree';

import { FakeServiceWorker } from '@/testing_tools/fakes/fake_service_worker';

import {
  ServiceWorkerState,
  ServiceWorkerStateInstance,
} from './service_worker_state';

describe('ServiceWorkerState', () => {
  let state: ServiceWorkerStateInstance;

  beforeEach(() => {
    state = ServiceWorkerState.create();
  });
  afterEach(() => {
    destroy(state);
  });

  test('should sync properties correctly', () => {
    const fakeSW = new FakeServiceWorker('installing', 'sw.js');
    const addEventListenerStub = jest.spyOn(fakeSW, 'addEventListener');
    const removeEventListenerStub = jest.spyOn(fakeSW, 'removeEventListener');
    addEventListenerStub.mockImplementation(() => {});
    removeEventListenerStub.mockImplementation(() => {});

    state.init(fakeSW);
    expect(state.serviceWorker).toStrictEqual(fakeSW);
    expect(state.state).toStrictEqual('installing');
    expect(addEventListenerStub.mock.calls.length).toStrictEqual(1);

    const [event, listener] = addEventListenerStub.mock.calls[0];
    expect(event).toStrictEqual('statechange');

    fakeSW.state = 'installed';
    if ('handleEvent' in listener) {
      listener.handleEvent(new Event('statechange'));
    } else {
      listener(new Event('statechange'));
    }

    expect(state.state).toStrictEqual('installed');
  });
});
