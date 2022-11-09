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

/* eslint-disable node/no-unpublished-import */

import * as sinon from 'sinon';

/**
 * A wrapper around `sinon.stub` that makes it easier to mock functions.
 *
 * Instead of
 * ```
 * sinon.stub<Parameters<typeof fnToStub>, ReturnType<typeof fnToStub>>();
 * ```
 *
 * users can write
 * ```
 * stubFn<typeof fnToStub>();
 * ```
 */
// `Parameters<T>` uses `any` in its type requirement.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function stubFn<T extends (...args: any) => any>() {
  return sinon.stub<Parameters<T>, ReturnType<T>>();
}

/**
 * A type alias for sinon.SinonStub that makes it easier to declare stubbed
 * functions.
 *
 * Instead of
 * ```
 * let stub: sinon.SinonStub<Parameters<typeof fnToStub>, ReturnType<typeof fnToStub>>;
 * ```
 *
 * users can write
 * ```
 * let stub: StubFn<typeof fnToStub>;
 * ```
 */
// `Parameters<T>` uses `any` in its type requirement.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type StubFn<T extends (...args: any) => any> = sinon.SinonStub<Parameters<T>, ReturnType<T>>;
