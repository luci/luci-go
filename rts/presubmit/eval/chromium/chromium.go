// Copyright 2020 The LUCI Authors.
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

package chromium

import "go.chromium.org/luci/rts/presubmit/eval"

// Backend implements eval.Backend for Chromium.
//
// It requires the developer to have BigQuery User role in
// "chrome-trooper-analytics" Cloud project.
type Backend struct{}

// Name implements eval.Backend.
func (*Backend) Name() string {
	return "chromium"
}

var _ eval.Backend = (*Backend)(nil)
