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

import { IObservableValue, observable } from 'mobx';

declare global {
  interface Promise<T> {
    /**
     * Converts a promise to an observable.
     */
    toObservable(): ObservablePromise<T>;
  }
  /**
   * Tagged type. Can be used to construct algebraic sum type.
   * Example:
   *  type Result<T, E> = Tagged<'ok', T> | Tagged<'err', E>;
   */
  interface Tagged<T, V> {
    tag: T;
    v: V;
  }
  type ObservablePromise<T> = IObservableValue<Tagged<'loading', null> | Tagged<'ok', T> | Tagged<'err', unknown>>;
}
Promise.prototype.toObservable = function<T>(this: Promise<T>): ObservablePromise<T> {
  const ret = observable.box({tag: 'loading', v: null}, {deep: false}) as ObservablePromise<T>;
  this.then((v) => ret.set({tag: 'ok', v}));
  this.catch((err) => ret.set({tag: 'err', v: err}));
  return ret;
};

declare global {
  interface Object {
    /**
     * Performs type assertion as a trailing method call.
     * Makes it easier to chain and type assertions with methods and property access.
     *
     * Example:
     *  ((grandParentWithLongName
     *    .parentWithLongName as SomeType)
     *    .childWithLongName
     *    .aCasualMethodCall() as SomeOtherType)
     *    .attribute;
     *
     * can be written as
     *  grandParent
     *    .parentWithLongName
     *    .as<SomeType>()
     *    .childWithLongName
     *    .aCasualMethodCall()
     *    .as<SomeOtherType>()
     *    .attribute;
     */
    as<T>(this: T): T;
  }
}
Object.prototype.as = function<T>(this: T) {
  return this;
};
