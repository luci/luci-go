// Copyright 2025 The LUCI Authors.
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

import { UseQueryOptions } from '@tanstack/react-query';
import { ReactNode } from 'react';

import { NonNullableProps } from '@/generic_libs/types';

/**
 * The definition of a single field (of a larger object).
 */
export interface FieldDef {
  readonly staticFields?: StaticFieldsSchema;
  readonly dynamicFields?: DynamicFieldsSchema;

  /**
   * A function that returns the values to be suggested to the users.
   */
  readonly getValues?: (partial: string) => readonly ValueDef[];
  /**
   * A function that returns the react-query to be used to retrieve value
   * suggestions.
   */
  // We don't care about the shape of the data returned from the `queryFn`.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  readonly fetchValues?: FetchValuesFn<any>;
}

/**
 * Defines the static fields (of a larger object).
 */
export interface StaticFieldsSchema {
  /**
   * A map of fields.
   *
   * Fields that includes the user input as a substring (case-insensitive) will
   * be suggested to the users.
   * Exact match (case-sensitive) are not suggested.
   */
  readonly [field: string]: FieldDef;
}

/**
 * Defines the dynamic fields (of a larger object).
 */
export interface DynamicFieldsSchema {
  readonly getKeys?: (partial: string) => readonly ValueDef[];
  readonly getValues?: (key: string, partial: string) => readonly ValueDef[];
}

/**
 * The definition of a single value (to be used in a field).
 */
export interface ValueDef {
  readonly text: string;
  readonly unselectable?: boolean;
  /**
   * HTML (React) representation of the value.
   *
   * Defaults to `text`.
   */
  readonly display?: ReactNode;
  /**
   * Explanation attached to this value.
   */
  readonly explanation?: ReactNode;
}

/**
 * A function that returns the react-query option to be used to retrieve value
 * suggestions.
 */
export type FetchValuesFn<T> = (partial: string) => ValueQueryOptions<T>;

/**
 * A react-query option to be used to retrieve value suggestions.
 */
export type ValueQueryOptions<T> =
  | Omit<UseQueryOptions<readonly ValueDef[]>, 'select'>
  // We don't care about the shape of the data returned from the `queryFn` when
  // `select` is specified.
  | NonNullableProps<UseQueryOptions<T, Error, readonly ValueDef[]>, 'select'>;

/**
 * An AIP-160 filter autocomplete suggestion
 */
export interface Suggestion {
  /**
   * Text representation of the suggestion.
   */
  readonly text: string;
  /**
   * HTML (React) representation of the suggestion.
   *
   * Defaults to `text`.
   */
  readonly display?: ReactNode;
  /**
   * Explanation attached to this suggestion.
   */
  readonly explanation?: ReactNode;
  /**
   * Apply the suggestion to the input.
   *
   * When called, returns the updated input and cursor position if the
   * suggestion is applied.
   */
  readonly apply: () => readonly [string, number];
}
