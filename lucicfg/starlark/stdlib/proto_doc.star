# Copyright 2018 The LUCI Authors.
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

# This module is not imported by anything. It exists only to document public
# native symbols exposed by `lucicfg` go code in the global namespace of all
# modules.


def _to_pbtext(msg):
  """Serializes a protobuf message to a string using ASCII proto serialization.

  Args:
    msg: a proto message to serialize. Required.
  """


def _to_jsonpb(msg):
  """Serializes a protobuf message to a string using JSONPB serialization.

  Args:
    msg: a proto message to serialize. Required.
  """


proto = struct(
    to_pbtext = _to_pbtext,
    to_jsonpb = _to_jsonpb,
)
