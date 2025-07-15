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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/validate"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// EmptyJSON corresponds to a serialized, empty JSON object.
const EmptyJSON = "{}"

// The maximum size of a variant, in bytes.
const maxVariantLength = 1024

// ValidateVariant returns an error if vr is invalid.
func ValidateVariant(vr *pb.Variant) error {
	if vr == nil {
		// N.B. In some legacy contexts the nil variant may still be valid
		// input. In those cases, ValidateVariant() should be called only
		// on non-nil variants.
		return validate.Unspecified()
	}
	for k, v := range vr.Def {
		p := pb.StringPair{Key: k, Value: v}
		if err := ValidateStringPair(&p, true); err != nil {
			return errors.Fmt("%q:%q: %w", k, v, err)
		}
	}
	if proto.Size(vr) > maxVariantLength {
		return errors.Fmt("got %v bytes; exceeds the maximum size of %d bytes", proto.Size(vr), maxVariantLength)
	}
	return nil
}

// Variant creates a pb.Variant from a list of strings alternating
// key/value. Does not validate pairs.
// See also VariantFromStrings.
//
// Panics if an odd number of tokens is passed.
func Variant(pairs ...string) *pb.Variant {
	if len(pairs)%2 != 0 {
		panic(fmt.Sprintf("odd number of tokens in %q", pairs))
	}

	vr := &pb.Variant{Def: make(map[string]string, len(pairs)/2)}
	for i := 0; i < len(pairs); i += 2 {
		vr.Def[pairs[i]] = pairs[i+1]
	}
	return vr
}

var nonNilEmptyStringSlice = []string{}

// VariantToStrings returns a key:val string slice representation of the Variant.
// Never returns nil.
func VariantToStrings(vr *pb.Variant) []string {
	if len(vr.GetDef()) == 0 {
		return nonNilEmptyStringSlice
	}

	keys := SortedVariantKeys(vr)
	pairs := make([]string, len(keys))
	defMap := vr.GetDef()
	for i, k := range keys {
		pairs[i] = fmt.Sprintf("%s:%s", k, defMap[k])
	}
	return pairs
}

// VariantFromStrings returns a Variant proto given the key:val string slice of its contents.
//
// If a key appears multiple times, the last pair wins.
func VariantFromStrings(pairs []string) (*pb.Variant, error) {
	if len(pairs) == 0 {
		return &pb.Variant{}, nil
	}

	def := make(map[string]string, len(pairs))
	for _, p := range pairs {
		pair, err := StringPairFromString(p)
		if err != nil {
			return nil, errors.Fmt("pair %q: %w", p, err)
		}
		def[pair.Key] = pair.Value
	}
	return &pb.Variant{Def: def}, nil
}

// SortedVariantKeys returns the keys in the variant as a sorted slice.
func SortedVariantKeys(vr *pb.Variant) []string {
	keys := make([]string, 0, len(vr.GetDef()))
	for k := range vr.GetDef() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// VariantHash returns a short hash of the variant.
func VariantHash(vr *pb.Variant) string {
	h := sha256.New()
	for _, k := range SortedVariantKeys(vr) {
		io.WriteString(h, k)
		io.WriteString(h, ":")
		io.WriteString(h, vr.Def[k])
		io.WriteString(h, "\n")
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}

// VariantToStringPairs returns a slice of StringPair derived from *pb.Variant.
func VariantToStringPairs(vr *pb.Variant) []*pb.StringPair {
	defMap := vr.GetDef()
	if len(defMap) == 0 {
		return nil
	}

	keys := SortedVariantKeys(vr)
	sp := make([]*pb.StringPair, len(keys))
	for i, k := range keys {
		sp[i] = StringPair(k, defMap[k])
	}
	return sp
}

// CombineVariant combines base variant and additional variant. The additional
// variant will overwrite the base variant if there is a duplicate key.
func CombineVariant(baseVariant *pb.Variant, additionalVariant *pb.Variant) *pb.Variant {
	if baseVariant == nil && additionalVariant == nil {
		return nil
	}

	variant := &pb.Variant{
		Def: make(map[string]string, len(baseVariant.GetDef())+len(additionalVariant.GetDef())),
	}
	for key, value := range baseVariant.GetDef() {
		variant.Def[key] = value
	}
	for key, value := range additionalVariant.GetDef() {
		variant.Def[key] = value
	}
	return variant
}

// VariantToJSON returns the JSON equivalent for a variant.
// Each key in the variant is mapped to a top-level key in the
// JSON object.
// e.g. `{"builder":"linux-rel","os":"Ubuntu-18.04"}`
func VariantToJSON(variant *pb.Variant) (string, error) {
	if variant == nil {
		// There is no string value we can send to BigQuery that
		// BigQuery will interpret as a NULL value for a JSON column:
		// - "" (empty string) is rejected as invalid JSON.
		// - "null" is interpreted as the JSON value null, not the
		//   absence of a value.
		// Consequently, the next best thing is to return an empty
		// JSON object.
		return EmptyJSON, nil
	}
	m := make(map[string]string)
	for key, value := range variant.Def {
		m[key] = value
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// VariantFromJSON convert json string representation of the variant into protocol buffer.
func VariantFromJSON(variant string) (*pb.Variant, error) {
	v := map[string]string{}
	if err := json.Unmarshal([]byte(variant), &v); err != nil {
		return nil, err
	}
	return &pb.Variant{Def: v}, nil
}
