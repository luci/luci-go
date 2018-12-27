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

package projectscope

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
)

const projectScopedServiceAccountPrefix = "ps-"

func GenerateAccountId(service, project string) string {
	structure := struct {
		Service string `json:"service"`
		Project string `json:"project"`
	}{
		Service: service,
		Project: project,
	}

	bytes, err := json.Marshal(&structure)
	if err != nil {
		panic("impossible to obtain error while hashing")
	}
	slice := sha256.Sum256(bytes)
	digest := hex.EncodeToString(slice[:])[0:27]
	result := projectScopedServiceAccountPrefix + digest
	if !IsValid(result) {
		panic(fmt.Errorf("generated accoundId should always be valid, is: %v", result))
	}
	return result
}

func IsValid(accountId string) bool {
	n := len(accountId)
	if n < 6 || n > 30 {
		return false
	}
	matched, err := regexp.Match("^[a-z]([-a-z0-9]*[a-z0-9])$", []byte(accountId))
	if err != nil {
		return false
	}
	return matched
}
