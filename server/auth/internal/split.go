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

package internal

import (
	"strings"
)

// SplitAuthHeader takes "Bearer <token>" and returns ("bearer", "<token>").
//
// If there's only one word, return ("", "<token>"). If there are more than two
// words, returns ("bearer", "<token> <more>").
//
// Strips spaces. Lower-cases the `typ`.
func SplitAuthHeader(header string) (typ string, token string) {
	parts := strings.SplitN(strings.TrimSpace(header), " ", 2)
	if len(parts) == 1 {
		return "", parts[0]
	}
	return strings.ToLower(strings.TrimSpace(parts[0])), strings.TrimSpace(parts[1])
}
