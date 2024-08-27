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

package app

import (
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/pubsub"
)

func errStatus(err error) string {
	if err == nil {
		return "success"
	}
	if transient.Tag.In(err) {
		return "transient-failure"
	} else if pubsub.Ignore.In(err) {
		return "ignored"
	} else {
		return "permanent-failure"
	}
}
