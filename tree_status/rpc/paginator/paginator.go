// Copyright 2024 The LUCI Authors.
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

// Package paginator contains helpers to convert between AIP-158 page_size/page_token/next_page_token fields
// and simple offset/limit values.
package paginator

import (
	"encoding/base64"
	"encoding/json"
	"hash/fnv"
	"sort"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
)

// Paginator converts between AIP-158 page_size/page_token/next_page_token fields
// and a simple offset/limit.
// Note that offset/limit is susceptible to page tearing during pagination if new
// items are added to the list during iteration.  This paginator makes no attempt
// to solve this at present, but can be extended to take additional state in the
// future to address this if necessary.
type Paginator struct {
	// The default page size to use if the user does not specify a page size.
	DefaultPageSize int32
	// The max page size to allow the user to request.
	MaxPageSize int32
}

// The structure that is encoded in the page token.
type token struct {
	// A hash of the request message to ensure all parameters are the same.
	Hash string
	// The offset of the next page.
	Offset int64
}

// Offset gets the offset given a request message with an AIP-158 page_token field.
func (p Paginator) Offset(request proto.Message) (int64, error) {
	// Make a copy of the request as we are going to mutate it.
	msg := request.ProtoReflect()
	field := msg.Descriptor().Fields().ByName("page_token")
	if field == nil {
		return 0, errors.New("request message does not have a page_token field")
	}
	tokenString := msg.Get(field).String()
	if tokenString == "" {
		return 0, nil
	}
	tokenBytes, err := base64.URLEncoding.DecodeString(tokenString)
	if err != nil {
		return 0, InvalidTokenError(err)
	}
	t := &token{}
	if err := json.Unmarshal(tokenBytes, &t); err != nil {
		return 0, InvalidTokenError(err)
	}
	if t.Hash != hashMessage(msg) {
		return 0, InvalidTokenError(errors.New("request message fields do not match page token"))
	}
	return t.Offset, nil
}

// NextPageToken gets the value to use for the next_page_token field in a response to remember the
// given offset for the next request.  The request message is required to ensure that
// the user does not change request parameters in the next request (hence making the
// offset invalid).
func (p Paginator) NextPageToken(request proto.Message, offset int64) (string, error) {
	t := &token{
		Hash:   hashMessage(request.ProtoReflect()),
		Offset: offset,
	}
	bytes, err := json.Marshal(t)
	if err != nil {
		return "", errors.Annotate(err, "creating next_page_token").Err()
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// Limit gets the limit given the page_size field and the configuration stored
// in the paginator.
func (p Paginator) Limit(requestedPageSize int32) int32 {
	if requestedPageSize <= 0 {
		requestedPageSize = p.DefaultPageSize
	}
	if requestedPageSize > p.MaxPageSize {
		requestedPageSize = p.MaxPageSize
	}
	return requestedPageSize
}

// hashMessage hashes all the fields in a request message except
// the AIP-158 page_size and page_token fields (which are allowed to change
// between requests).
func hashMessage(msg protoreflect.Message) string {
	keys := []string{}
	values := map[string]string{}
	msg.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		name := fd.Name()
		if name == "page_token" || name == "page_size" {
			return true
		}
		keys = append(keys, string(name))
		values[string(name)] = v.String()
		return true
	})
	hash := fnv.New64a()
	sortedKeys := sort.StringSlice(keys)
	for _, key := range sortedKeys {
		hash.Write([]byte(key))
		hash.Write([]byte(values[key]))
	}
	return strconv.FormatUint(hash.Sum64(), 36)
}

// InvalidTokenError annotates the error with InvalidArgument appstatus.
func InvalidTokenError(err error) error {
	return appstatus.Attachf(err, codes.InvalidArgument, "invalid page_token")
}
