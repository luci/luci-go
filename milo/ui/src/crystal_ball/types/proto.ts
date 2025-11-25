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
 * google.protobuf.Timestamp
 */
export interface Timestamp {
  /**
   * Represents seconds of UTC time since Unix epoch.
   */
  seconds?: number;

  /**
   * Non-negative fractions of a second at nanosecond resolution.
   */
  nanos?: number;
}

/**
 * google.protobuf.Struct
 * Internal interface used by the exported Value type.
 */
interface Struct {
  [key: string]: Value;
}

/**
 * google.protobuf.Value
 */
export type Value = null | number | string | boolean | Struct | Array<Value>;
