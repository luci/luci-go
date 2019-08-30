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

# wellknown_descpb is proto.DescriptorSet with following protos:
#   load("@proto//google/protobuf/any.proto", any_pb="google.protobuf")
#   load("@proto//google/protobuf/duration.proto", duration_pb="google.protobuf")
#   load("@proto//google/protobuf/empty.proto", empty_pb="google.protobuf")
#   load("@proto//google/protobuf/struct.proto", struct_pb="google.protobuf")
#   load("@proto//google/protobuf/timestamp.proto", timestamp_pb="google.protobuf")
#   load("@proto//google/protobuf/wrappers.proto", wrappers_pb="google.protobuf")
wellknown_descpb = __native__.wellknown_descpb

# googtypes_descpb is proto.DescriptorSet with following files:
#   load("@proto//google/type/dayofweek.proto", dayofweek_pb="google.type")
googtypes_descpb = __native__.googtypes_descpb
