// Copyright 2022 The LUCI Authors.
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

// Package controllegacy provides methods to read and write records used to:
//  1. Ensure exactly-once ingestion of test results from builds.
//  2. Synchronise build completion and presubmit run completion, so that
//     ingestion only proceeds when both build and presubmit run have
//     completed.
package controllegacy
