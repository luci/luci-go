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

// Package quotakeys has utility functions for generating internal quota Redis
// keys.
package quotakeys

import (
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/server/quota/quotapb"
)

const (
	// QuotaFieldDelim is used to delimit sections of keys which are user provided values.
	//
	// NOTE: this is ascii85-safe.
	QuotaFieldDelim = "~"

	// redisKeyPrefix is the prefix used used by ALL Redis keys in this module.
	//
	// It'll be repeated many times in the db, so we picked `"a` (i.e quote-ah)...
	// Unique? Short? Cute? Obscure? Yes.
	redisKeyPrefix = `"a`

	accountKeyPrefix      = redisKeyPrefix + QuotaFieldDelim + "a" + QuotaFieldDelim
	policyConfigPrefix    = redisKeyPrefix + QuotaFieldDelim + "p" + QuotaFieldDelim
	requestDedupKeyPrefix = redisKeyPrefix + QuotaFieldDelim + "r" + QuotaFieldDelim
)

// parseRedisKey will parse a quota library redis key, and populate the given
// Key proto message with its contents.
//
// This function assumes that the key proto has N string fields which have the
// proto field numbers 1..N.
func parseRedisKey(key, prefix string, to proto.Message) error {
	if !strings.HasPrefix(key, prefix) {
		return errors.New("incorrect prefix")
	}
	toks := strings.Split(key[len(prefix):], QuotaFieldDelim)
	p := to.ProtoReflect()
	fields := p.Descriptor().Fields()
	if fields.Len() != len(toks) {
		return errors.Reason("incorrect number of sections: %d != %d", len(toks), fields.Len()).Err()
	}

	for i, tok := range toks {
		fn := protoreflect.FieldNumber(i + 1)
		fd := fields.ByNumber(fn)
		// HACK: Support uint for PolicyConfigID
		if fd.Kind() == protoreflect.Uint32Kind {
			val, err := strconv.ParseUint(tok, 10, 32)
			if err != nil {
				return errors.Annotate(err, "bad version scheme").Err()
			}
			p.Set(fd, protoreflect.ValueOfUint32(uint32(val)))
		} else {
			// assume string
			p.Set(fd, protoreflect.ValueOfString(tok))
		}
	}

	return to.(interface{ ValidateAll() error }).ValidateAll()
}

// serializeRedisKey generates
func serializeRedisKey(prefix string, from proto.Message) string {
	p := from.ProtoReflect()
	fields := p.Descriptor().Fields()
	fLen := fields.Len()
	bld := strings.Builder{}
	bld.WriteString(prefix)
	for fNum := protoreflect.FieldNumber(1); int(fNum) <= fLen; fNum++ {
		if fNum > 1 {
			bld.WriteString(QuotaFieldDelim)
		}
		bld.WriteString(p.Get(fields.ByNumber(fNum)).String())
	}
	return bld.String()
}

// ParseAccountKey parses a raw key string and extracts a AccountID
// from it (or returns an error).
func ParseAccountKey(key string) (*quotapb.AccountID, error) {
	ret := &quotapb.AccountID{}
	err := parseRedisKey(key, accountKeyPrefix, ret)
	return ret, err
}

// AccountKey returns the full redis key for an account.
func AccountKey(id *quotapb.AccountID) string {
	return serializeRedisKey(accountKeyPrefix, id)
}

// ParsePolicyConfigID parses a raw key string and extracts a PolicyConfigID
// from it (or returns an error).
func ParsePolicyConfigID(policyConfigID string) (*quotapb.PolicyConfigID, error) {
	ret := &quotapb.PolicyConfigID{}
	err := parseRedisKey(policyConfigID, policyConfigPrefix, ret)
	return ret, err
}

// PolicyConfigID returns a full redis key for the given PolicyConfigID.
//
// `id` must already be validated, or this could panic.
func PolicyConfigID(id *quotapb.PolicyConfigID) string {
	return serializeRedisKey(policyConfigPrefix, id)
}

// ParsePolicyKey parses a raw key string and extracts a PolicyKey
// from it (or returns an error).
func ParsePolicyKey(policyKey string) (*quotapb.PolicyKey, error) {
	ret := &quotapb.PolicyKey{}
	err := parseRedisKey(policyKey, "", ret)
	return ret, err
}

// PolicyKey returns a full redis key for the given PolicyKey.
//
// `id` must already be validated, or this could panic.
func PolicyKey(id *quotapb.PolicyKey) string {
	return serializeRedisKey("", id)
}

// ParsePolicyRef parses a raw PolicyRef and extracts a PolicyID
// from it (or returns an error).
func ParsePolicyRef(ref *quotapb.PolicyRef) (ret *quotapb.PolicyID, err error) {
	ret = &quotapb.PolicyID{}
	ret.Config, err = ParsePolicyConfigID(ref.Config)
	if err == nil {
		ret.Key, err = ParsePolicyKey(ref.Key)
	}
	return ret, err
}

// PolicyRef returns a PolicyRef for the given PolicyID.
//
// `id` must already be validated, or this could panic.
func PolicyRef(id *quotapb.PolicyID) *quotapb.PolicyRef {
	return &quotapb.PolicyRef{
		Config: PolicyConfigID(id.Config),
		Key:    PolicyKey(id.Key),
	}
}

// ParseRequestDedupKey parses a raw key string and extracts the userID and requestID
// from the request key.
func ParseRequestDedupKey(key string) (*quotapb.RequestDedupKey, error) {
	ret := &quotapb.RequestDedupKey{}
	err := parseRedisKey(key, requestDedupKeyPrefix, ret)
	return ret, err
}

// RequestDedupKey returns the full redis key for a request dedup entry.
//
// Args:
//   - userID is the luci auth identity of the requestor.
//   - requestID is the user's provided requestID.
//
// Example (`RequestDedupKey("user:user@example.com", "something")`):
//
//	"a~r~user:user@example.com~something
//
// Returns a request deduplication key.
//
// None of the arguments may contain "~".
func RequestDedupKey(id *quotapb.RequestDedupKey) string {
	return serializeRedisKey(requestDedupKeyPrefix, id)
}
