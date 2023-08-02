# Copyright 2019 The LUCI Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Exposes descriptor sets with some built-in well-known protos."""

# wellknown_descpb is proto.DescriptorSet with following files:
#   load('@proto//google/protobuf/any.proto', any_pb='google.protobuf')
#   load('@proto//google/protobuf/duration.proto', duration_pb='google.protobuf')
#   load('@proto//google/protobuf/descriptor.proto', descriptor_pb='google.protobuf')
#   load('@proto//google/protobuf/empty.proto', empty_pb='google.protobuf')
#   load('@proto//google/protobuf/field_mask.proto', field_mask_pb='google.protobuf')
#   load('@proto//google/protobuf/struct.proto', struct_pb='google.protobuf')
#   load('@proto//google/protobuf/timestamp.proto', timestamp_pb='google.protobuf')
#   load('@proto//google/protobuf/wrappers.proto', wrappers_pb='google.protobuf')
wellknown_descpb = __native__.wellknown_descpb

# googtypes_descpb is proto.DescriptorSet with following files:
#   load('@proto//google/type/calendar_period.proto', calendar_period_pb='google.type')
#   load('@proto//google/type/color.proto', color_pb='google.type')
#   load('@proto//google/type/date.proto', date_pb='google.type')
#   load('@proto//google/type/dayofweek.proto', dayofweek_pb='google.type')
#   load('@proto//google/type/expr.proto', expr_pb='google.type')
#   load('@proto//google/type/fraction.proto', fraction_pb='google.type')
#   load('@proto//google/type/latlng.proto', latlng_pb='google.type')
#   load('@proto//google/type/money.proto', money_pb='google.type')
#   load('@proto//google/type/postal_address.proto', postal_address_pb='google.type')
#   load('@proto//google/type/quaternion.proto', quaternion_pb='google.type')
#   load('@proto//google/type/timeofday.proto', timeofday_pb='google.type')
googtypes_descpb = __native__.googtypes_descpb

# annotations_descpb is proto.DescriptorSet with following files:
#   load('@proto//google/api/annotations.proto', annotations_pb='google.api')
#   load('@proto//google/api/client.proto', client_pb='google.api')
#   load('@proto//google/api/field_behavior.proto', field_behavior_pb='google.api')
#   load('@proto//google/api/http.proto', http_pb='google.api')
#   load('@proto//google/api/resource.proto', resource_pb='google.api')
annotations_descpb = __native__.annotations_descpb

# validation_descpb is a proto.DescriptorSet with following files:
#   load('@proto//validation/validation.proto', validation_pb='validate')
# This is github.com/envoyproxy/protoc-gen-validate protos.
validation_descpb = __native__.validation_descpb
