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

package flag

import (
	"flag"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/grpc/metadata"
)

// metadataFlag implements the flag.Getter returned by GRPCMetadata.
type grpcMetadataFlag metadata.MD

// GRPCMetadata returns a flag.Getter for parsing gRPC metadata from a
// a set of colon-separated strings.
// Example: -f a:1 -f a:2 -f b:3
// Panics if md is nil.
func GRPCMetadata(md metadata.MD) flag.Getter {
	if md == nil {
		panic("md is nil")
	}
	return grpcMetadataFlag(md)
}

// String implements the flag.Value interface.
func (f grpcMetadataFlag) String() string {
	pairs := make([]string, 0, len(f))
	for k, vs := range f {
		for _, v := range vs {
			pairs = append(pairs, fmt.Sprintf("%s:%s", k, v))
		}
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ", ")
}

// Set implements the flag.Value interface.
func (f grpcMetadataFlag) Set(s string) error {
	parts := strings.Split(s, ":")
	if len(parts) == 1 {
		return fmt.Errorf("no colon")
	}
	metadata.MD(f).Append(parts[0], parts[1])
	return nil
}

// Get retrieves the flag value.
func (f grpcMetadataFlag) Get() any {
	return metadata.MD(f)
}
