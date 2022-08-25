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

package clustering

import "regexp"

// ChunkRe matches validly formed chunk IDs.
var ChunkRe = regexp.MustCompile(`^[0-9a-f]{1,32}$`)

// AlgorithmRePattern is the regular expression pattern matching
// validly formed clustering algorithm names.
// The overarching requirement is [0-9a-z\-]{1,32}, which we
// sudivide into an algorithm name of up to 26 characters
// and an algorithm version number.
const AlgorithmRePattern = `[0-9a-z\-]{1,26}-v[1-9][0-9]{0,3}`

// AlgorithmRe matches validly formed clustering algorithm names.
var AlgorithmRe = regexp.MustCompile(`^` + AlgorithmRePattern + `$`)
