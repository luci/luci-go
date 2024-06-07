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

export interface TimeZoneConfig {
  label: string;
  zone: string;
}

export const DEFAULT_EXTRA_ZONE_CONFIGS: ReadonlyArray<TimeZoneConfig> = [
  {
    label: 'PT',
    zone: 'America/Los_Angeles',
  },
  {
    label: 'UTC',
    zone: 'utc',
  },
];
