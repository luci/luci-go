// Copyright 2026 The LUCI Authors.
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
 * Prefix used to identify dimensions originating from Swarming bots.
 * This is used in URLs, UI columns, and maps to `swarming_labels` in the backend DB.
 */
export const BROWSER_SWARMING_SOURCE = 'sw';

/**
 * Prefix used to identify metadata originating from UFS/Machine database.
 * This is used in URLs, UI columns, and maps to `ufs_labels` in the backend DB.
 */
export const BROWSER_UFS_SOURCE = 'ufs';
