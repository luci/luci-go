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

import { reaction } from 'mobx';
import { addDisposer, Instance, types } from 'mobx-state-tree';

import { ServiceWorkerState } from './service_worker_state';

/**
 * A utility type that makes some ServiceWorkerRegistration properties
 * observable.
 */
export const ServiceWorkerRegistrationState = types
  .model('ServiceWorkerRegistrationState', {
    id: types.optional(types.identifierNumber, () => Math.random()),
    installing: types.maybe(ServiceWorkerState),
    waiting: types.maybe(ServiceWorkerState),
  })
  .volatile(() => ({
    registration: null as ServiceWorkerRegistration | null,
  }))
  .actions((self) => ({
    _set(key: 'installing' | 'waiting', serviceWorker: ServiceWorker | null) {
      if (!serviceWorker) {
        self[key] = undefined;
        return;
      }

      const state = ServiceWorkerState.create();
      self[key] = state;
      state.init(serviceWorker);
    },
    init(registration: ServiceWorkerRegistration) {
      if (self.registration) {
        throw new Error('already initialized');
      }
      self.registration = registration;
      this._set('installing', registration.installing);
      this._set('waiting', registration.waiting);

      const onUpdateFound = () => {
        this._set('installing', registration.installing);
      };
      registration.addEventListener('updatefound', onUpdateFound);
    },
    afterCreate() {
      addDisposer(
        self,
        // Keep track of the installing service worker state.
        reaction(
          () => [self.installing?.serviceWorker, self.installing?.state] as const,
          ([sw]) => {
            if (!sw) {
              return;
            }
            if (sw.state !== 'installing') {
              this._set('installing', null);
            }
            if (sw.state === 'installed') {
              this._set('waiting', sw);
            }
          },
          { fireImmediately: true }
        )
      );

      addDisposer(
        self,
        // Keep track of the waiting service worker state.
        reaction(
          () => [self.waiting?.serviceWorker, self.waiting?.state] as const,
          ([sw]) => {
            if (!sw) {
              return;
            }
            if (sw.state !== 'installed') {
              this._set('waiting', null);
            }
          },
          { fireImmediately: true }
        )
      );
    },
  }));

export type ServiceWorkerRegistrationStateInstance = Instance<typeof ServiceWorkerRegistrationState>;
