// Copyright 2023 The LUCI Authors.
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

type Constructor<T, P extends unknown[] = []> = new (...params: P) => T;

/**
 * `varName: NoInfer<T>` makes TSC unable to be inferred `T` from the value
 * assigned to `varName`.
 *
 * For example:
 * ```
 * // Without `NoInfer<T>`.
 * function foo<T = never>(param: T): T {
 *   return param;
 * }
 *
 * // `T` is inferred to be `1`.
 * foo(1);
 *
 * // With `NoInfer<T>`.
 * // Note that `T = never` is needed. Otherwise the un-inferable `T` will
 * // default to `unknown`, therefore making the type restriction too loose.
 * function bar<T = never>(param: NoInfer<T>): T {
 *   return param;
 * }
 *
 * // The type of T cannot be inferred, therefore fallback to the default type
 * // (i.e. `never`). As a result, TSC will report the following error:
 * // Argument of type 'number' is not assignable to parameter of type 'never'.
 * bar(1);
 *
 * // The type has to be specified explicitly.
 * bar<number>(1);
 * ```
 */
// `[T][T extends unknown ? 0 : never]` is a noop because it always evaluate to
// `T`. However, TSC defers the evaluation of unresolved conditional types,
// making the default type (explicitly specified or `unknown`) take precedence
// over the inferred type.
type NoInfer<T> = [T][T extends unknown ? 0 : never];

/**
 * Any key that has an associated function in `S`.
 */
type FunctionKeys<S> = keyof {
  // The request type has to be `any` because the argument type must be contra-
  // variant when sub-typing a function.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  [K in keyof S as S[K] extends (...params: any[]) => unknown
    ? K
    : never]: S[K];
};

type Mutable<T> = {
  -readonly [key in keyof T]: T[key];
};

type DeepMutable<T> = {
  -readonly [key in keyof T]: DeepMutable<T[key]>;
};

type NonNullableProps<T, K extends keyof T = keyof T> = Omit<T, K> & {
  [key in K]-?: NonNullable<T[key]>;
};

type DeepNonNullable<T> = T extends object
  ? NonNullable<{
      [key in keyof T]-?: DeepNonNullable<T[key]>;
    }>
  : NonNullable<T>;

type DeepNonNullableProps<T, K extends keyof T = keyof T> = Omit<T, K> & {
  [key in K]-?: DeepNonNullable<T[key]>;
};

interface ToString {
  toString(): string;
}

type Result<T, E> = ResultOk<T> | ResultErr<E>;

interface ResultOk<T> {
  ok: true;
  value: T;
}

interface ResultErr<E> {
  ok: false;
  value: E;
}
