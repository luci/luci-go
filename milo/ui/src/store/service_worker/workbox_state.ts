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

import { types } from 'mobx-state-tree';
import { Workbox } from 'workbox-window';

import { aliveFlow } from '../../libs/milo_mobx_utils';
import { getEnv } from '../env';
import { ServiceWorkerRegistrationState } from './service_worker_registration_state';

/**
 * A utility type that makes some Workbox properties observable.
 */
export const WorkboxState = types
  .model('WorkboxState', {
    id: types.optional(types.identifierNumber, () => Math.random()),
    registration: types.maybe(ServiceWorkerRegistrationState),
  })
  .views((self) => ({
    get hasPendingUpdate() {
      return Boolean(self.registration?.waiting);
    },
  }))
  .volatile(() => ({
    workbox: null as Workbox | null,
  }))
  .actions((self) => {
    return {
      /**
       * Initialize a Workbox instance and register it.
       */
      init: aliveFlow(self, function* (url: TrustedScriptURL | string) {
        if (self.workbox) {
          throw new Error('already initialized');
        }
        self.workbox = new Workbox(url, { type: getEnv(self).isDevEnv ? 'module' : 'classic' });
        const call = self.workbox.register();

        // .register() always resolves a ServiceWorkerRegistration on the first
        // call.
        const registration: NonNullable<Awaited<typeof call>> = yield call;

        const registrationState = ServiceWorkerRegistrationState.create();
        registrationState.init(registration);
        self.registration = registrationState;
      }),
    };
  });
