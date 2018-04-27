// Copyright 2018 The LUCI Authors.
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

package gitiles

// These constants are possible values for RefsRequest.RefsPath field.
// Not an exhaustive list.
const (
	// AllRefs instructs the client to fetch all refs.
	AllRefs = "refs"
	// Branches instructs the client to fetch all branches.
	Branches = "refs/heads"
	// Tags instructs the client to fetch all tags.
	Tags = "refs/tags"
)
