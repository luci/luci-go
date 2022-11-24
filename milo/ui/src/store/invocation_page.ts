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
import { addDisposer, Instance, SnapshotIn, SnapshotOut, types } from 'mobx-state-tree';

import { InvocationState } from './invocation_state';
import { ServicesStore } from './services';

export const InvocationPage = types
  .model('InvocationPage', {
    services: types.safeReference(ServicesStore),

    invocationId: types.maybe(types.string),

    invocation: types.optional(InvocationState, {}),
  })
  .actions((self) => ({
    setDependencies(deps: Partial<Pick<typeof self, 'services'>>) {
      Object.assign<typeof self, Partial<typeof self>>(self, deps);
    },
    setInvocationId(invId: string) {
      self.invocationId = invId;
    },
    afterCreate() {
      addDisposer(
        self,
        reaction(
          () => self.services,
          (services) => {
            self.invocation.setDependencies({
              services,
            });
          },
          { fireImmediately: true }
        )
      );

      self.invocation.setDependencies({
        invocationIdGetter: () => self.invocationId || null,
      });
    },
  }));

export type InvocationPageInstance = Instance<typeof InvocationPage>;
export type InvocationPageSnapshotIn = SnapshotIn<typeof InvocationPage>;
export type InvocationPageSnapshotOut = SnapshotOut<typeof InvocationPage>;
