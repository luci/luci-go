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

// We define a custom key that is used to allow specific queries to opt into
// IndexedDB caching through useQuery. This is used by use cases like caching
// our DeviceDimensions data.
// IndexedDB caching enables us to show data to users faster on initial page
// load but runs into risks like increased deserialization time and introducing
// caching related edge cases, so we want to opt-in specific queries where
// the extra benefits might be worth the increased code complexity and risk.
export const PERSIST_INDEXED_DB = 'persist-to-indexeddb';

export const DEFAULT_IDB_STORE_NAME = 'FLEET_CONSOLE_DB';
