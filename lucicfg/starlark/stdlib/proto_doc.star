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

"""Some extra documentation.

This module is not imported by anything. It exists only to document public
native symbols exposed by `lucicfg` go code in the global namespace of all
modules.
"""

def _to_textpb(msg):
    """Serializes a protobuf message to a string using ASCII proto serialization.

    Args:
      msg: a proto message to serialize. Required.
    """
    _unused(msg)

def _to_jsonpb(msg, use_proto_names = None):
    """Serializes a protobuf message to a string using JSONPB serialization.

    Args:
      msg: a proto message to serialize. Required.
      use_proto_names: boolean, whether to use snake_case in field names
        instead of camelCase. The default is False.
    """
    _unused(msg, use_proto_names)

def _to_wirepb(msg):
    """Serializes a protobuf message to a string using binary wire encoding.

    Args:
      msg: a proto message to serialize. Required.
    """
    _unused(msg)

def _from_textpb(ctor, text):
    """Deserializes a protobuf message given its ASCII proto serialization.

    Args:
      ctor: a message constructor function, the same one you would normally use
        to create a new message. Required.
      text: a string with the serialized message. Required.

    Returns:
      Deserialized message constructed via `ctor`.
    """
    _unused(ctor, text)

def _from_jsonpb(ctor, text):
    """Deserializes a protobuf message given its JSONPB serialization.

    Args:
      ctor: a message constructor function, the same one you would normally use
        to create a new message. Required.
      text: a string with the serialized message. Required.

    Returns:
      Deserialized message constructed via `ctor`.
    """
    _unused(ctor, text)

def _from_wirepb(ctor, blob):
    """Deserializes a protobuf message given its wire serialization.

    Args:
      ctor: a message constructor function, the same one you would normally use
        to create a new message. Required.
      blob: a string with the serialized message. Required.

    Returns:
      Deserialized message constructed via `ctor`.
    """
    _unused(ctor, blob)

def _struct_to_textpb(s):
    """Converts a struct to a text proto string.

    Args:
      s: a struct object. May not contain dicts.

    Returns:
      A str containing a text format protocol buffer message.
    """
    _unused(s)

def _clone(msg):
    """Returns a deep copy of a given proto message.

    Args:
      msg: a proto message to make a copy of. Required.

    Returns:
      A deep copy of the message.
    """
    _unused(msg)

def _has(msg, field):
    """Checks if a proto message has the given optional field set.

    Following rules apply:

      * Fields that are not defined in the `*.proto` file are always unset.
      * Singular fields of primitive types (e.g. `int64`), repeated and map
        fields (even empty ones) are always set. There's no way to distinguish
        zero values of such fields from unset fields.
      * Singular fields of message types are set only if they were explicitly
        initialized (e.g. by writing to such field or reading a default value
        from it).
      * Alternatives of a `oneof` field (regardless of their type) are
        initialized only when they are explicitly "picked".

    Args:
      msg: a message to check. Required.
      field: a string name of the field to check. Required.

    Returns:
      True if the message has the field set.
    """
    _unused(msg, field)

def _unused(*args):  # @unused
    """Used exclusively to shut up `unused-variable` lint.

    DocTags:
      Hidden.
    """

proto = struct(
    to_textpb = _to_textpb,
    to_jsonpb = _to_jsonpb,
    to_wirepb = _to_wirepb,
    from_textpb = _from_textpb,
    from_jsonpb = _from_jsonpb,
    from_wirepb = _from_wirepb,
    struct_to_textpb = _struct_to_textpb,
    clone = _clone,
    has = _has,
)
