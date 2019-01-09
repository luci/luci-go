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
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"

	"go.chromium.org/luci/common/logging"
)

const (
	encodedProjectLength = 10
)

// digest implements the specific hash function to derive account id suffixes of constant lenght 8.
//
// See utils_test.go for canonical input/output examples
func digest(service, project string) [8]byte {
	encoder := base64.StdEncoding.WithPadding(base64.NoPadding)
	encodedService := encoder.EncodeToString([]byte(service))
	encodedProject := encoder.EncodeToString([]byte(project))
	slice := sha256.Sum256([]byte(encodedService + "|" + encodedProject))
	var result [8]byte
	copy(result[:], slice[0:8])
	return result
}

// encode performs a GCP service account id compliant encoding on a LUCI project name.
//
// No assumptions are made about the format of the LUCI project name to assert future compatibility.
// See utils_test.go for canonical input/output examples
func encode(project string) [encodedProjectLength]byte {
	res := [encodedProjectLength]byte{}
	for i := range res {
		res[i] = '-'
	}
	copy(res[:], project)
	return res
}

// GenerateAccountId creates a unique account id suited for a project scope service account.
//
// The accountid is deterministic depending on the (service, project) pair.
func GenerateAccountId(service, project string) string {
	//slice := sha256.Sum256(encode(service, project))
	//digest := hex.EncodeToString(slice[:])[0:27]
	//result := projectScopedServiceAccountPrefix + projectScopedServiceAccountDilimeter encode(service, project) + string(digest)

	d := digest(service, project)
	result := fmt.Sprintf("ps-%s-%s", encode(project), hex.EncodeToString(d[:]))
	if !IsValid(result) {
		logging.WithError(fmt.Errorf("generated invalid account id: %v", result))
		panic(result)
	}
	return result
}

// IsValid checks whether a given account id is valid to be used with the GCP IAM service accounts API.
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
