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

import { IPromiseBasedObservable, PENDING, REJECTED } from 'mobx-utils';

/**
 * Unwraps the value in a promise based observable.
 *
 * If the observable is pending, return the defaultValue.
 * If the observable is rejected, throw the error.
 */
export function unwrapObservable<T>(observable: IPromiseBasedObservable<T>, defaultValue: T) {
  switch (observable.state) {
    case PENDING:
      return defaultValue;
    case REJECTED:
      throw observable.value;
    default:
      return observable.value;
  }
}
