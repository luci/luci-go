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

/**
 * The number of milliseconds in a second.
 */
export const SECOND_MS = 1000;

/**
 * The number of milliseconds in a minute.
 */
export const MINUTE_MS = 60000;

/**
 * The number of milliseconds in an hour.
 */
export const HOUR_MS = 3600000;

/**
 * A list of numbers in ascending order that are suitable to be used as time
 * intervals (in ms).
 */
export const PREDEFINED_TIME_INTERVALS = Object.freeze([
  // Values that can divide 10ms.
  1, // 1ms
  5, // 5ms
  10, // 10ms
  // Values that can divide 100ms.
  20, // 20ms
  25, // 25ms
  50, // 50ms
  // Values that can divide 1 second.
  100, // 100ms
  125, // 125ms
  200, // 200ms
  250, // 250ms
  500, // 500ms
  // Values that can divide 15 seconds.
  1000, // 1s
  2000, // 2s
  3000, // 3s
  5000, // 5s
  // Values that can divide 1 minute.
  10000, // 10s
  15000, // 15s
  20000, // 20s
  30000, // 30s
  60000, // 1min
  // Values that can divide 15 minutes.
  120000, // 2min
  180000, // 3min
  300000, // 5min
  // Values that can divide 1 hour.
  600000, // 10min
  900000, // 15min
  1200000, // 20min
  1800000, // 30min
  3600000, // 1hr
  // Values that can divide 12 hours.
  7200000, // 2hr
  10800000, // 3hr
  14400000, // 4hr
  21600000, // 6hr
  // Values that can divide 1 day.
  28800000, // 8hr
  43200000, // 12hr
  86400000, // 24hr
]);
