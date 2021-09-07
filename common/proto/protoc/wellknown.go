// Copyright 2021 The LUCI Authors.
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

package protoc

// Well-known Google proto packages -> go packages they are implemented in.
var googlePackages = map[string]string{
	"google/type/color.proto":          "google.golang.org/genproto/googleapis/type/color",
	"google/type/date.proto":           "google.golang.org/genproto/googleapis/type/date",
	"google/type/dayofweek.proto":      "google.golang.org/genproto/googleapis/type/dayofweek",
	"google/type/latlng.proto":         "google.golang.org/genproto/googleapis/type/latlng",
	"google/type/money.proto":          "google.golang.org/genproto/googleapis/type/money",
	"google/type/postal_address.proto": "google.golang.org/genproto/googleapis/type/postaladdress",
	"google/type/timeofday.proto":      "google.golang.org/genproto/googleapis/type/timeofday",

	"google/protobuf/any.proto":        "google.golang.org/protobuf/types/known/anypb",
	"google/protobuf/descriptor.proto": "google.golang.org/protobuf/types/descriptorpb",
	"google/protobuf/duration.proto":   "google.golang.org/protobuf/types/known/durationpb",
	"google/protobuf/empty.proto":      "google.golang.org/protobuf/types/known/emptypb",
	"google/protobuf/struct.proto":     "google.golang.org/protobuf/types/known/structpb",
	"google/protobuf/timestamp.proto":  "google.golang.org/protobuf/types/known/timestamppb",
	"google/protobuf/wrappers.proto":   "google.golang.org/protobuf/types/known/wrapperspb",

	"google/rpc/code.proto":          "google.golang.org/genproto/googleapis/rpc/code",
	"google/rpc/error_details.proto": "google.golang.org/genproto/googleapis/rpc/errdetails",
	"google/rpc/status.proto":        "google.golang.org/genproto/googleapis/rpc/status",

	"google/api/annotations.proto":    "google.golang.org/genproto/googleapis/api/annotations",
	"google/api/http.proto":           "google.golang.org/genproto/googleapis/api/annotations",
	"google/api/field_behavior.proto": "google.golang.org/genproto/googleapis/api/annotations",
	"google/api/resource.proto":       "google.golang.org/genproto/googleapis/api/annotations",
}

// packageMap returns a mapping "proto package path => go package path".
//
// Maps well-known protos to corresponding packages. `custom` allows to add more
// mappings or override default ones.
func packageMap(custom map[string]string) map[string]string {
	if len(custom) == 0 {
		return googlePackages
	}
	combined := make(map[string]string, len(custom)+len(googlePackages))
	for k, v := range custom {
		combined[k] = v
	}
	for k, v := range googlePackages {
		if _, ok := combined[k]; !ok {
			combined[k] = v
		}
	}
	return combined
}
