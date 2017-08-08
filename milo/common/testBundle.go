// Copyright 2016 The LUCI Authors.
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

// Data Structures used for defining test data for use with testing each theme.

package common

import (
	"go.chromium.org/luci/server/templates"
)

// TestBundle is a template arg associated with a description used for testing.
type TestBundle struct {
	// Description is a short one line description of what the data contains.
	Description string
	// Data is the data fed directly into the template.
	Data templates.Args
}
