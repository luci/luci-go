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

package pbutil

import (
	"context"
	"encoding/base64"
	"fmt"
	"os/user"
	"strings"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

const (
	invocationIDPattern      = `[a-z][a-z0-9_\-:.]{0,99}`
	invalidInvocationIDChars = `[^a-z0-9_\-:.]`
	maxInvocationIDLength    = 100
)

var (
	invocationIDRe             = regexpf("^%s$", invocationIDPattern)
	invalidInvocationIDCharsRE = regexpf(invalidInvocationIDChars)
	invocationNameRe           = regexpf("^invocations/(%s)$", invocationIDPattern)
)

// ValidateInvocationID returns a non-nil error if id is invalid.
func ValidateInvocationID(id string) error {
	return validateWithRe(invocationIDRe, id)
}

// ValidateInvocationName returns a non-nil error if name is invalid.
func ValidateInvocationName(name string) error {
	_, err := ParseInvocationName(name)
	return err
}

// ParseInvocationName extracts the invocation id.
func ParseInvocationName(name string) (id string, err error) {
	if name == "" {
		return "", unspecified()
	}

	m := invocationNameRe.FindStringSubmatch(name)
	if m == nil {
		return "", doesNotMatch(invocationNameRe)
	}
	return m[1], nil
}

// InvocationName synthesizes an invocation name from an id.
// Does not validate id, use ValidateInvocationID.
func InvocationName(id string) string {
	return "invocations/" + id
}

// NormalizeInvocation converts inv to the canonical form.
func NormalizeInvocation(inv *pb.Invocation) {
	sortStringPairs(inv.Tags)
}

// GenerateTestInvocationID generates a test invocation ID that is rare enough to be unique
// globally.
//
// The ID is made of the current username, timestamp in a human-friendly format with
// a random suffix so that it's easy to find the creator and creation time.
func GenerateTestInvocationID(ctx context.Context) (string, error) {
	whoami, err := user.Current()
	if err != nil {
		return "", err
	}
	bytes := make([]byte, 8)
	if _, err := mathrand.Read(ctx, bytes); err != nil {
		return "", err
	}

	username := strings.ToLower(whoami.Username)
	username = invalidInvocationIDCharsRE.ReplaceAllString(username, "")
	suffix := strings.ToLower(fmt.Sprintf(
		"%s:%s", clock.Now(ctx).UTC().Format(time.RFC3339),
		// Use unpadded encoding because the padding character,'=', is not a valid character
		// for invocation IDs and also shorter.
		base64.RawURLEncoding.EncodeToString(bytes)))

	return fmt.Sprintf("u:%.*s:%s", maxInvocationIDLength-len(suffix), username, suffix), nil
}
