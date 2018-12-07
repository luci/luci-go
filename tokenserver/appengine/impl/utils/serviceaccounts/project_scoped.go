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
	"fmt"
	"regexp"
)

// shorten transforms any str longer than max into a string of length up to max.
func shorten(str string, max int) string {
	if max < 0 {
		return ""
	}
	if len(str) <= max {
		return str
	}
	hexdigest := fmt.Sprintf("%x", sha256.Sum256([]byte(str)))
	return fmt.Sprintf("%s%s", str[0:max/2], hexdigest[0:max/2])
}

func GenerateAccountEmail(service, project string) (string, error) {
	if service == "" || project == "" {
		return "", fmt.Errorf("Parameters must not be empty")
	}

	generated := fmt.Sprintf("%s-%s", shorten(service, 14), shorten(project, 14))
	if !IsValid(generated) {
		panic(fmt.Errorf("Generated account id should always be valid"))
	}
	return generated, nil
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
