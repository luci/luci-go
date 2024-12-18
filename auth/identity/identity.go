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

package identity

import (
	"fmt"
	"regexp"
	"strings"
)

// Kind is enumeration of known identity kinds. See Identity.
type Kind string

const (
	// Anonymous kind means no identity information is provided. Identity value
	// is always 'anonymous'.
	Anonymous Kind = "anonymous"

	// Bot is used for bots authenticated via IP allowlist. Used primarily by
	// Swarming. Identity value encodes bot ID.
	Bot Kind = "bot"

	// Project is used to convey that the request from one LUCI project to another
	// is being made on behalf of some LUCI project. Identity value is project ID.
	Project Kind = "project"

	// Service is used for GAE apps using X-Appengine-Inbound-Appid header for
	// authentication. Identity value is GAE app id.
	Service Kind = "service"

	// User is used for regular users. Identity value is email address.
	User Kind = "user"
)

// KnownKinds is mapping between Kind and regexp for identity values.
//
// See also appengine/components/components/auth/model.py in luci-py.
var KnownKinds = map[Kind]*regexp.Regexp{
	Anonymous: regexp.MustCompile(`^anonymous$`),
	Bot:       regexp.MustCompile(`^[0-9a-zA-Z_\-\.@]+$`),
	Project:   regexp.MustCompile(`^[a-z0-9\-_]+$`),
	Service:   regexp.MustCompile(`^[0-9a-zA-Z_\-\:\.]+$`),
	User:      regexp.MustCompile(`^[0-9a-zA-Z_\-\.\+\%]+@[0-9a-zA-Z_\-\.]+$`),
}

// Identity represents a caller that makes requests. A string of the form
// "kind:value" where 'kind' is one of Kind constant and meaning of 'value'
// depends on a kind (see comments for Kind values).
type Identity string

// NormalizedIdentity is similar to Identity, but for identities with 'kind'
// User, the 'value' component has been normalized. This is primarily to support
// case-insensitive checks.
type NormalizedIdentity string

// AnonymousIdentity represents anonymous user.
const AnonymousIdentity Identity = "anonymous:anonymous"

// MakeIdentity ensures 'identity' string looks like a valid identity and
// returns it as Identity value.
func MakeIdentity(identity string) (Identity, error) {
	id := Identity(identity)
	if err := id.Validate(); err != nil {
		return "", err
	}
	return id, nil
}

// Validate checks that the identity string is well-formed.
func (id Identity) Validate() error {
	chunks := strings.SplitN(string(id), ":", 2)
	if len(chunks) != 2 {
		return fmt.Errorf("auth: bad identity string %q", id)
	}
	re := KnownKinds[Kind(chunks[0])]
	if re == nil {
		return fmt.Errorf("auth: bad identity kind %q", chunks[0])
	}
	if !re.MatchString(chunks[1]) {
		return fmt.Errorf("auth: bad value %q for identity kind %q", chunks[1], chunks[0])
	}
	return nil
}

// Kind returns identity kind. If identity string is invalid returns Anonymous.
func (id Identity) Kind() Kind {
	chunks := strings.SplitN(string(id), ":", 2)
	if len(chunks) != 2 {
		return Anonymous
	}
	return Kind(chunks[0])
}

// Value returns a valued encoded in the identity, e.g. for User identity kind
// it is user's email address. If identity string is invalid returns "anonymous".
func (id Identity) Value() string {
	chunks := strings.SplitN(string(id), ":", 2)
	if len(chunks) != 2 {
		return "anonymous"
	}
	return chunks[1]
}

// Email returns user's email for identity with kind User or empty string for
// all other identity kinds. If identity string is undefined returns "".
func (id Identity) Email() string {
	chunks := strings.SplitN(string(id), ":", 2)
	if len(chunks) != 2 {
		return ""
	}
	if Kind(chunks[0]) == User {
		return chunks[1]
	}
	return ""
}

// AsNormalized returns the Identity as a NormalizedIdentity.
func (id Identity) AsNormalized() NormalizedIdentity {
	return NewNormalizedIdentity(string(id))
}

// NewNormalizedIdentity creates a NormalizedIdentity from the given 'identity'
// string. Note: the returned NormalizedIdentity may be an invalid identity,
// as no validation occurs.
func NewNormalizedIdentity(identity string) NormalizedIdentity {
	return NormalizedIdentity(normalize(identity))
}

// normalize applies normalization to the given 'identity' string.
func normalize(identity string) string {
	if strings.HasPrefix(identity, "user:") {
		// Normalization is simply making the 'value' component all lowercase,
		// but this implementation could change.
		return strings.ToLower(identity)
	}

	// Not a User identity so return as-is.
	return identity
}
