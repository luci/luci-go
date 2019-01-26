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
	// encodedProjectLength resembles the maximum number of characters to which
	// the luci project will be encoded in the generated account id.
	encodedProjectLength = 10

	// AccountIDDelimiter is the delimiter between account id and domain part
	// of the service account email.
	AccountIDDelimiter = "@"
)

var (
	// regex for service account ID API compliance validation.
	regex = regexp.MustCompile("^[a-z]([-a-z0-9]*[a-z0-9])$")
)

// digest implements the specific hash function to derive account id suffixes of constant length 8.
//
// See utils_test.go for canonical input/output examples
func digest(service, project string) []byte {
	encoder := base64.StdEncoding.WithPadding(base64.NoPadding)
	encodedService := encoder.EncodeToString([]byte(service))
	encodedProject := encoder.EncodeToString([]byte(project))
	sha256digest := sha256.Sum256([]byte(encodedService + "|" + encodedProject))
	return sha256digest[:8]
}

// encode performs a GCP service account id compliant encoding on a LUCI project name.
//
// No assumptions are made about the format of the LUCI project name to assert future compatibility.
// See utils_test.go for canonical input/output examples
func encode(project string) []byte {
	res := [encodedProjectLength]byte{}
	copy(res[:], project)
	if len(project) < encodedProjectLength {
		return res[:len(project)]
	}
	return res[:]
}

// GenerateAccountID creates a unique account email prefix suited for a project scope service account.
//
// The account id is deterministic depending on the (service, project) pair.
func GenerateAccountID(service, project string) string {
	d := digest(service, project)
	result := fmt.Sprintf("ps-%s-%s", encode(project), hex.EncodeToString(d[:]))
	if !IsValid(result) {
		logging.WithError(fmt.Errorf("generated invalid account id: %v", result))
		panic(result)
	}
	return result
}

// IsValid checks whether a given account id is valid to be used with the GCP IAM service accounts API.
func IsValid(accountID string) bool {
	n := len(accountID)
	if n < 6 || n > 30 {
		return false
	}
	return regex.Match([]byte(accountID))
}
