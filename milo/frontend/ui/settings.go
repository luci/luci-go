// Copyright 2015 The LUCI Authors.
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

package ui

// Settings denotes a full renderable Milo settings page.
type Settings struct {
	// Where the form should go.
	ActionURL string

	// Themes is a list of usable themes for Milo
	Theme *Choices
}

// Choices - A dropdown menu showing all possible choices.
type Choices struct {
	// A list of all possible choices.
	Choices []string

	// The selected choice.
	Selected string
}
