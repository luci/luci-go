// Copyright 2019 The LUCI Authors.
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

package main

import (
	"time"
)

const (
	day  = 24 * time.Hour
	week = 7 * day

	// Delete Invocations row after this duration since invocation creation.
	invocationExpirationDuration = 2 * 365 * day // 2 y

	// Delete expected test results afte this duration since invocation creation.
	expectedTestResultsExpirationDuration = 60 * day // 2mo

	// By default, interrupt the invocation 1h after creation if it is still
	// incomplete.
	defaultInvocationDeadlineDuration = time.Hour
)
