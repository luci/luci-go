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

// Package bootstrapResult defines a common way to express the result of
// bootstrapping a command via JSON.
package bootstrapResult

import (
	"encoding/json"
	"os"
)

// Result is the bootstrap result data.
type Result struct {
	// ReturnCode is the process' return code.
	ReturnCode int `json:"return_code"`
	// Command, is present, is the bootstrapped command that was run.
	Command []string `json:"command,omitempty"`
}

// WriteJSON writes Result as a JSON document to the specified path.
func (r *Result) WriteJSON(path string) error {
	fd, err := os.Create(path)
	if err != nil {
		return err
	}
	defer fd.Close()
	return json.NewEncoder(fd).Encode(r)
}
