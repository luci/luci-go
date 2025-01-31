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

"""Exports proto modules with messages used in generated LUCI configs.

Prefer using this module over loading "@proto//..." modules directly. Proto
paths may change in a backward incompatible way. Using this module gives more
stability.
"""

load("@stdlib//internal/luci/descpb.star", "lucitypes_descpb")

lucitypes_descpb.register()

load("@proto//go.chromium.org/luci/buildbucket/proto/common.proto", _common_pb = "buildbucket.v2")
load("@proto//go.chromium.org/luci/buildbucket/proto/project_config.proto", _buildbucket_pb = "buildbucket")
load("@proto//go.chromium.org/luci/common/proto/config/project_config.proto", _config_pb = "config")
load("@proto//go.chromium.org/luci/common/proto/realms/realms_config.proto", _realms_pb = "auth_service")
load("@proto//go.chromium.org/luci/cv/api/config/v2/config.proto", _cq_pb = "cv.config")
load("@proto//go.chromium.org/luci/cv/api/v1/run.proto", _cv_v1pb = "cv.v1")
load("@proto//go.chromium.org/luci/logdog/api/config/svcconfig/project.proto", _logdog_pb = "svcconfig")
load("@proto//go.chromium.org/luci/logdog/api/config/svcconfig/cloud_logging.proto", _logdog_cloud_logging_pb = "svcconfig")
load("@proto//go.chromium.org/luci/luci_notify/api/config/notify.proto", _notify_pb = "notify")
load("@proto//go.chromium.org/luci/milo/proto/projectconfig/project.proto", _milo_pb = "luci.milo.projectconfig")
load("@proto//go.chromium.org/luci/resultdb/proto/v1/invocation.proto", _resultdb_pb = "luci.resultdb.v1")
load("@proto//go.chromium.org/luci/resultdb/proto/v1/predicate.proto", _predicate_pb = "luci.resultdb.v1")
load("@proto//go.chromium.org/luci/scheduler/appengine/messages/config.proto", _scheduler_pb = "scheduler.config")

buildbucket_pb = _buildbucket_pb
common_pb = _common_pb
config_pb = _config_pb
cq_pb = _cq_pb
cv_v1pb = _cv_v1pb
logdog_pb = _logdog_pb
logdog_cloud_logging_pb = _logdog_cloud_logging_pb
milo_pb = _milo_pb
notify_pb = _notify_pb
predicate_pb = _predicate_pb
realms_pb = _realms_pb
resultdb_pb = _resultdb_pb
scheduler_pb = _scheduler_pb
