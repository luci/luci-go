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

import { addDisposer, Instance, types } from 'mobx-state-tree';

/**
 * A utility type that makes some ServiceWorker properties observable.
 */
export const ServiceWorkerState = types
  .model('ServiceWorkerState', {
    id: types.optional(types.identifierNumber, () => Math.random()),
    state: types.maybe(types.frozen<ServiceWorkerState>()),
  })
  .volatile(() => ({
    serviceWorker: null as ServiceWorker | null,
  }))
  .actions((self) => ({
    _setState(value: ServiceWorkerState) {
      self.state = value;
    },
    init(serviceWorker: ServiceWorker) {
      if (self.serviceWorker) {
        throw new Error('already initialized');
      }
      self.serviceWorker = serviceWorker;
      this._setState(serviceWorker.state);

      const onStateChange = () => this._setState(serviceWorker.state);
      addDisposer(self, () => serviceWorker.removeEventListener('statechange', onStateChange));
      serviceWorker.addEventListener('statechange', onStateChange);
    },
  }));

export type ServiceWorkerStateInstance = Instance<typeof ServiceWorkerState>;
