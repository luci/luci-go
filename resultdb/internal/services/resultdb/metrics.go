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

package resultdb

import (
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/field"
)

var (
	// queryInvocationsCount tracks the number of invocations provided in
	// resultDBServer query methods.
	queryInvocationsCount = metric.NewCounter(
		"resultdb/rpc/query_invocations_count",
		"The number of valid calls to query methods",
		nil,
		// The name of the method being called, e.g. "QueryTestVariants"
		field.String("method"),
		// The number of invocations in the request
		field.Int("invocations_count"),
	)
)
