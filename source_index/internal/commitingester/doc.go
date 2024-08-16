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

// Package commitingester contains methods for handling commit ingestion.
//
// It is able to ingest all commits reachable from a given commitish using the
// following steps.
//  1. Retrieve up to N closest commits reachable from the given commitish and
//     page token.
//  2. Check whether those commits has already been saved into the database.
//  3. If yes, stops the ingestion.
//  4. If not, save those commits into the database.
//  5. If root commit hasn't been reached, transactionally schedule another
//     commit ingestion task with the next page token.
//
// Because the ingestion stops when it encounters a commit that is already
// ingested, it's important to maintain the following constraints:
//   - If a commit is saved into the database, all its ancestors must also have
//     been saved into the database.
package commitingester
