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

/**
 * InvocationID represents an AnTS Invocation ID.
 */
export interface InvocationID {
  readonly branch: string;
  readonly target: string;
  readonly buildProvider: string;
  readonly scheduler: string;
  readonly testName: string;
  readonly properties?: Record<string, string>;
}

/**
 * TestID represents an AnTS (partial) test ID.
 */
export interface TestID {
  readonly module: string;
  readonly moduleParameters?: Record<string, string>;
  readonly testClass: string;
  readonly method: string;
  readonly aggregationLevel: string;
}

export const AggregationLevel = {
  INVOCATION: 'INVOCATION',
  MODULE: 'MODULE',
  PACKAGE: 'PACKAGE',
  CLASS: 'CLASS',
  METHOD: 'METHOD',
  UNSPECIFIED: 'AGGREGATION_LEVEL_UNSPECIFIED',
} as const;
