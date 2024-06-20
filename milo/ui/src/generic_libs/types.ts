// Copyright 2024 The LUCI Authors.
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

export type Constructor<T, P extends unknown[] = []> = new (...params: P) => T;

/**
 * Any key that has an associated function in `S`.
 */
export type FunctionKeys<S> = keyof {
  // The request type has to be `any` because the argument type must be contra-
  // variant when sub-typing a function.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  [K in keyof S as S[K] extends (...params: any[]) => unknown
    ? K
    : never]: S[K];
};

export type ArrayElement<Arr> = Arr extends ReadonlyArray<infer T> ? T : never;

export type Mutable<T> = {
  -readonly [key in keyof T]: T[key];
};

export type DeepMutable<T> = {
  -readonly [key in keyof T]: DeepMutable<T[key]>;
};

export type NonNullableProps<T, K extends keyof T = keyof T> = Omit<T, K> & {
  [key in K]-?: NonNullable<T[key]>;
};

export type DeepNonNullable<T> = T extends object
  ? NonNullable<{
      [key in keyof T]-?: DeepNonNullable<T[key]>;
    }>
  : NonNullable<T>;

export type DeepNonNullableProps<T, K extends keyof T = keyof T> = Omit<
  T,
  K
> & {
  [key in K]-?: DeepNonNullable<T[key]>;
};

export interface ToString {
  toString(): string;
}

export type Result<T, E> = ResultOk<T> | ResultErr<E>;

export interface ResultOk<T> {
  ok: true;
  value: T;
}

export interface ResultErr<E> {
  ok: false;
  value: E;
}
