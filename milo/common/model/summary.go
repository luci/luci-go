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

package model

import "time"

// Summary summarizes a thing (step, build, group of builds, whatever).
type Summary struct {
	// Status indicates the 'goodness' and lifetime of the thing. This usually
	// translates directly to a status color.
	Status Status

	// Start indicates when this thing started doing its action.
	Start time.Time

	// End indicates when this thing completed doing its action.
	End time.Time

	// Text is a possibly-multi-line summary of what happened.
	Text []string
}
