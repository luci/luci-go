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

# lucitypes_descpb is proto.DescriptorSet with following files:
#   load('@stdlib//internal/luci/proto.star', 'buildbucket_pb')
#   load('@stdlib//internal/luci/proto.star', 'config_pb')
#   load('@stdlib//internal/luci/proto.star', 'cq_pb')
#   load('@stdlib//internal/luci/proto.star', 'logdog_pb')
#   load('@stdlib//internal/luci/proto.star', 'milo_pb')
#   load('@stdlib//internal/luci/proto.star', 'notify_pb')
#   load('@stdlib//internal/luci/proto.star', 'realms_pb')
#   load('@stdlib//internal/luci/proto.star', 'resultdb_pb')
#   load('@stdlib//internal/luci/proto.star', 'scheduler_pb')
lucitypes_descpb = __native__.lucitypes_descpb
