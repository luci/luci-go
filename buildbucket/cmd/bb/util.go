// Copyright 2019 The LUCI Authors.
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

package main

import (
	"fmt"
	"strings"
)

// confirm interactively asks the user to confirm a prompt.
func confirm(prompt string, defaultReply string) string {
	fmt.Printf("%s ", prompt)
	if defaultReply != "" {
		fmt.Printf("[%s] ", defaultReply)
	}

	var s string
	fmt.Scanln(&s)
	if strings.TrimSpace(s) == "" {
		s = defaultReply
	}
	return s
}
