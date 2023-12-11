// Copyright 2022 The LUCI Authors.
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

// Package pbutil contains methods for manipulating LUCI Analysis protos.
package pbutil

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	cvv0 "go.chromium.org/luci/cv/api/v0"

	pb "go.chromium.org/luci/analysis/proto/v1"
)

// EmptyJSON corresponds to a serialized, empty JSON object.
const EmptyJSON = "{}"
const maxStringPairKeyLength = 64
const maxStringPairValueLength = 256
const stringPairKeyPattern = `[a-z][a-z0-9_]*(/[a-z][a-z0-9_]*)*`

var stringPairKeyRe = regexp.MustCompile(fmt.Sprintf(`^%s$`, stringPairKeyPattern))
var stringPairRe = regexp.MustCompile(fmt.Sprintf("(?s)^(%s):(.*)$", stringPairKeyPattern))
var variantHashRe = regexp.MustCompile("^[0-9a-f]{16}$")

// ProjectRePattern is the regular expression pattern that matches
// validly formed LUCI Project names.
// From https://source.chromium.org/chromium/infra/infra/+/main:luci/appengine/components/components/config/common.py?q=PROJECT_ID_PATTERN
const ProjectRePattern = `[a-z0-9\-]{1,40}`

// projectRe matches validly formed LUCI Project names.
var projectRe = regexp.MustCompile(`^` + ProjectRePattern + `$`)

// MustTimestampProto converts a time.Time to a *timestamppb.Timestamp and panics
// on failure.
func MustTimestampProto(t time.Time) *timestamppb.Timestamp {
	ts := timestamppb.New(t)
	if err := ts.CheckValid(); err != nil {
		panic(err)
	}
	return ts
}

// AsTime converts a *timestamppb.Timestamp to a time.Time.
func AsTime(ts *timestamppb.Timestamp) (time.Time, error) {
	if ts == nil {
		return time.Time{}, errors.Reason("unspecified").Err()
	}
	if err := ts.CheckValid(); err != nil {
		return time.Time{}, err
	}
	return ts.AsTime(), nil
}

func doesNotMatch(r *regexp.Regexp) error {
	return errors.Reason("does not match %s", r).Err()
}

// StringPair creates a pb.StringPair with the given strings as key/value field values.
func StringPair(k, v string) *pb.StringPair {
	return &pb.StringPair{Key: k, Value: v}
}

// StringPairs creates a slice of pb.StringPair from a list of strings alternating key/value.
//
// Panics if an odd number of tokens is passed.
func StringPairs(pairs ...string) []*pb.StringPair {
	if len(pairs)%2 != 0 {
		panic(fmt.Sprintf("odd number of tokens in %q", pairs))
	}

	strpairs := make([]*pb.StringPair, len(pairs)/2)
	for i := range strpairs {
		strpairs[i] = StringPair(pairs[2*i], pairs[2*i+1])
	}
	return strpairs
}

// StringPairFromString creates a pb.StringPair from the given key:val string.
func StringPairFromString(s string) (*pb.StringPair, error) {
	m := stringPairRe.FindStringSubmatch(s)
	if m == nil {
		return nil, doesNotMatch(stringPairRe)
	}
	return StringPair(m[1], m[3]), nil
}

// StringPairToString converts a StringPair to a key:val string.
func StringPairToString(pair *pb.StringPair) string {
	return fmt.Sprintf("%s:%s", pair.Key, pair.Value)
}

// StringPairsToStrings converts pairs to a slice of "{key}:{value}" strings
// in the same order.
func StringPairsToStrings(pairs ...*pb.StringPair) []string {
	ret := make([]string, len(pairs))
	for i, p := range pairs {
		ret[i] = StringPairToString(p)
	}
	return ret
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

// VariantFromStrings returns a Variant proto given the key:val string slice of its contents.
//
// If a key appears multiple times, the last pair wins.
func VariantFromStrings(pairs []string) (*pb.Variant, error) {
	if len(pairs) == 0 {
		return nil, nil
	}

	def := make(map[string]string, len(pairs))
	for _, p := range pairs {
		pair, err := StringPairFromString(p)
		if err != nil {
			return nil, errors.Annotate(err, "pair %q", p).Err()
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
		pairs[i] = (k + ":" + defMap[k])
	}
	return pairs
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

// PresubmitRunModeFromString returns a pb.PresubmitRunMode corresponding
// to a CV Run mode string.
func PresubmitRunModeFromString(mode string) (pb.PresubmitRunMode, error) {
	switch mode {
	case "FULL_RUN":
		return pb.PresubmitRunMode_FULL_RUN, nil
	case "DRY_RUN":
		return pb.PresubmitRunMode_DRY_RUN, nil
	case "QUICK_DRY_RUN":
		return pb.PresubmitRunMode_QUICK_DRY_RUN, nil
	case "NEW_PATCHSET_RUN":
		return pb.PresubmitRunMode_NEW_PATCHSET_RUN, nil
	}
	return pb.PresubmitRunMode_PRESUBMIT_RUN_MODE_UNSPECIFIED, fmt.Errorf("unknown run mode %q", mode)
}

// PresubmitRunStatusFromLUCICV returns a pb.PresubmitRunStatus corresponding
// to a LUCI CV Run status. Only statuses corresponding to an ended run
// are supported.
func PresubmitRunStatusFromLUCICV(status cvv0.Run_Status) (pb.PresubmitRunStatus, error) {
	switch status {
	case cvv0.Run_SUCCEEDED:
		return pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_SUCCEEDED, nil
	case cvv0.Run_FAILED:
		return pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_FAILED, nil
	case cvv0.Run_CANCELLED:
		return pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_CANCELED, nil
	}
	return pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_UNSPECIFIED, fmt.Errorf("unknown run status %q", status)
}

func ValidateProject(project string) error {
	if project == "" {
		return errors.Reason("unspecified").Err()
	}
	if !projectRe.MatchString(project) {
		return errors.Reason("must match %s", projectRe).Err()
	}
	return nil
}
