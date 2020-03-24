/* Copyright 2020 The LUCI Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import { deepStripNotIn } from './deep_strip_not_in';

declare global {
  namespace Chai {
    interface Assert {
      /**
       * Asserts that haystack includes needle. Comparing to deepInclude,
       * uses recursiveDeepInclude instead of deepEqual when comparing
       * elements in a (nested) property of Array type.
       *
       * @type T   Type of haystack.
       * @param haystack   Object.
       * @param needle   Potential subset of the haystack's properties.
       * @param message   Message to display on error.
       */
      // any has to be used here to express that we don't want T to be WeakSet
      // or Array of any type.
      // tslint:disable-next-line: no-any
      recursiveDeepInclude<T>(haystack: T, needle: T extends WeakSet<any> | any[] ? never : Partial<T>, message?: string): void;
    }
  }
}

export const chaiRecursiveDeepInclude: Chai.ChaiPlugin = (chai) => {
  chai.assert.recursiveDeepInclude = (actual, expected, msg) => {
    chai.assert.deepInclude(deepStripNotIn(actual, expected), expected, msg);
  };
};
