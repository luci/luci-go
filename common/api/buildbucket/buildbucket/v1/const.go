// Copyright 2017 The LUCI Authors.
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

package buildbucket

// Build statuses.
const (
	// StatusScheduled means a build was created, but did not start or
	// complete.
	// The initial state of a build.
	StatusScheduled = "SCHEDULED"
	// StatusStarted means a build has started.
	StatusStarted = "STARTED"
	// StatusCompleted means a build has completed. See its Result.
	StatusCompleted = "COMPLETED"
)

// Build status filters.
// Can be used only when searching.
// A build cannot have any of these statuses.
//
// Any build status defined above can be used as a status filter too.
const (
	// StatusFilterIncomplete matches StatusScheduled or StatusStarted.
	StatusFilterIncomplete = "INCOMPLETE"
)

const (
	// ResultFailure means a build has failed, with or without an infra-failure.
	ResultFailure = "FAILURE"
	// ResultSuccess means a build has succeeded.
	ResultSuccess = "SUCCESS"
	// ResultCanceled means a build was cancelled or timed out.
	ResultCanceled = "CANCELED"
)

const (
	// ReasonNotFound means the given build ID was not found on the BuildBucket service.
	ReasonNotFound = "BUILD_NOT_FOUND"
)
