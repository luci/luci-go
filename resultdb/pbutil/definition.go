// Copyright 2025 The LUCI Authors.
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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"sort"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/validate"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

var (
	// rootInvocationDefSystemRE restricts the system name to kebab-case.
	rootInvocationDefSystemRE = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z][a-z0-9]*)*$`)
)

const (
	maxRootInvocationDefSystemLength   = 64   // bytes
	maxRootInvocationDefNameLength     = 256  // bytes
	maxRootInvocationDefPropertiesSize = 1024 // bytes
)

// ValidateDefinitionForStorage validates the given root invocation definition
// is valid for storage.
// It returns an error if the definition is invalid.
func ValidateDefinitionForStorage(def *pb.RootInvocationDefinition) error {
	if def == nil {
		return errors.New("unspecified")
	}

	if err := validate.MatchReWithLength(rootInvocationDefSystemRE, 1, maxRootInvocationDefSystemLength, def.System); err != nil {
		return errors.Fmt("system: %w", err)
	}

	if def.Name == "" {
		return errors.New("name: unspecified")
	}
	if err := ValidateUTF8PrintableStrict(def.Name, maxRootInvocationDefNameLength); err != nil {
		return errors.Fmt("name: %w", err)
	}

	// This is a method validating the definition for storage, not for queries,
	// so we always require def.Properties to be set.
	if err := ValidateDefinitionProperties(def.Properties); err != nil {
		return errors.Fmt("properties: %w", err)
	}
	return nil
}

// ValidateDefinitionProperties validates the given root invocation definition properties.
// It returns an error if the properties are invalid.
func ValidateDefinitionProperties(props *pb.RootInvocationDefinition_Properties) error {
	if props == nil {
		return validate.Unspecified()
	}

	for k, v := range props.Def {
		p := pb.StringPair{Key: k, Value: v}
		if err := ValidateStringPair(&p, true); err != nil {
			return errors.Fmt("def[%q]: %w", k, err)
		}
	}

	totalSize := proto.Size(props)
	if totalSize > maxRootInvocationDefPropertiesSize {
		return errors.Fmt("got %v bytes; exceeds the maximum size of %d bytes", totalSize, maxRootInvocationDefPropertiesSize)
	}
	return nil
}

// DefinitionProperties creates a pb.RootInvocationDefinition_Properties from a list of strings alternating
// key/value. Does not validate pairs.
// See also RootInvocationDefinitionPropertiesFromStrings.
//
// Panics if an odd number of tokens is passed.
func DefinitionProperties(pairs ...string) *pb.RootInvocationDefinition_Properties {
	if len(pairs)%2 != 0 {
		panic(fmt.Sprintf("odd number of tokens in %q", pairs))
	}

	p := &pb.RootInvocationDefinition_Properties{Def: make(map[string]string, len(pairs)/2)}
	for i := 0; i < len(pairs); i += 2 {
		p.Def[pairs[i]] = pairs[i+1]
	}
	return p
}

// SortedDefinitionPropertiesKeys returns the keys in the properties as a sorted slice.
func SortedDefinitionPropertiesKeys(p *pb.RootInvocationDefinition_Properties) []string {
	keys := make([]string, 0, len(p.GetDef()))
	for k := range p.GetDef() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// RootInvocationDefinitionProperties returns a key:val string slice representation of the root invocation definition properties.
// Never returns nil.
func RootInvocationDefinitionPropertiesToStrings(p *pb.RootInvocationDefinition_Properties) []string {
	if len(p.GetDef()) == 0 {
		return nonNilEmptyStringSlice
	}

	keys := SortedDefinitionPropertiesKeys(p)
	pairs := make([]string, len(keys))
	defMap := p.GetDef()
	for i, k := range keys {
		pairs[i] = fmt.Sprintf("%s:%s", k, defMap[k])
	}
	return pairs
}

// DefinitionPropertiesFromStrings returns a RootInvocationDefinition_Properties proto given
// the key:val string slice of its contents.
//
// If a key appears multiple times, an error is returned.
func DefinitionPropertiesFromStrings(pairs []string) (*pb.RootInvocationDefinition_Properties, error) {
	if len(pairs) == 0 {
		return &pb.RootInvocationDefinition_Properties{}, nil
	}

	def := make(map[string]string, len(pairs))
	for _, p := range pairs {
		pair, err := StringPairFromString(p)
		if err != nil {
			return nil, errors.Fmt("pair %q: %w", p, err)
		}
		if _, ok := def[pair.Key]; ok {
			return nil, errors.Fmt("duplicate key %q", pair.Key)
		}
		def[pair.Key] = pair.Value
	}
	return &pb.RootInvocationDefinition_Properties{Def: def}, nil
}

// DefinitionPropertiesHash computes the hash of a set of
// RootInvocationDefinition properties.
func DefinitionPropertiesHash(props *pb.RootInvocationDefinition_Properties) string {
	if props == nil {
		return ""
	}
	h := sha256.New()
	for _, k := range SortedDefinitionPropertiesKeys(props) {
		io.WriteString(h, k)
		io.WriteString(h, ":")
		io.WriteString(h, props.Def[k])
		io.WriteString(h, "\n")
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}

// PopulateDefinitionHashes populates the hashes of the given root invocation definition.
// This is used to ensure that the hashes are consistent with the definition.
func PopulateDefinitionHashes(d *pb.RootInvocationDefinition) {
	d.PropertiesHash = DefinitionPropertiesHash(d.Properties)
}
