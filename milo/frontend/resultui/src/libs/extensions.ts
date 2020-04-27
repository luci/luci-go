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

// Indicates that this is a module. See ts(2669).
export {};

declare global {
  interface Array<T> {
    /**
     * The last element in the array.
     */
    readonly last: T | undefined;
  }
}
Object.defineProperty(Array.prototype, 'last', {
  get(this: unknown[]) {
    return this[this.length - 1];
  },
});
