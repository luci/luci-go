// Copyright 2020 The LUCI Authors.
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

declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
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
      recursiveDeepInclude<T>(
        haystack: T,
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        needle: T extends WeakSet<any> | any[] ? never : Partial<T>,
        message?: string
      ): void;
    }
  }
}

export const chaiRecursiveDeepInclude: Chai.ChaiPlugin = (chai) => {
  chai.assert.recursiveDeepInclude = (actual, expected, msg) => {
    chai.assert.deepEqual(deepStripNotIn(actual, expected), expected, msg);
  };
};

/**
 * Removes properties and nested properties in source that are not in
 * propertyTree.
 * This function returns a new object and does not modify source.
 *
 * @throw If the source or source's (nested) property is an array,
 * propertyTree or propertyTree's corresponding (nested) property should be an
 * array of the same length. Otherwise an error will be thrown.
 */
function deepStripNotIn(source: unknown, propertyTree: unknown) {
  if (!(source instanceof Object && propertyTree instanceof Object)) {
    return source;
  }

  if (source instanceof Array) {
    if (!(propertyTree instanceof Array)) {
      throw new Error('incompatible type. actual is an array but propertyTree is not.');
    }
    if (source.length !== propertyTree.length) {
      throw new Error('actual and propertyTree must have the same length');
    }
    const ret: unknown[] = [];
    for (let i = 0; i < propertyTree.length; ++i) {
      ret.push(deepStripNotIn(source[i], propertyTree[i]));
    }
    return ret;
  }

  const ret: { [key: string]: unknown } = {};
  for (const key of Object.keys(propertyTree)) {
    if (key in source) {
      ret[key] = deepStripNotIn(
        (source as { [key: string]: unknown })[key],
        (propertyTree as { [key: string]: unknown })[key]
      );
    }
  }
  return ret;
}
