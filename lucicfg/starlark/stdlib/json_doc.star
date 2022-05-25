# Copyright 2020 The LUCI Authors.
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

def _json_encode(value):
    """Encodes a value into a JSON string.

    Accepts one required positional argument, which it converts to JSON by
    cases:

      - None, True, and False are converted to null, true, and false,
        respectively.
      - Starlark int values, no matter how large, are encoded as decimal
        integers. Some decoders may not be able to decode very large integers.
      - Starlark float values are encoded using decimal point notation, even if
        the value is an integer. It is an error to encode a non-finite
        floating-point value.
      - Starlark strings are encoded as JSON strings, using UTF-16 escapes.
      - a Starlark IterableMapping (e.g. dict) is encoded as a JSON object.
        It is an error if any key is not a string. The order of keys is
        retained.
      - any other Starlark Iterable (e.g. list, tuple) is encoded as a JSON
        array.
      - a Starlark HasAttrs (e.g. struct) is encoded as a JSON object.

    Encoding any other value yields an error.

    Args:
      value: a value to encode. Required.
    """
    _unused(value)

def _json_decode(str):
    """Decodes a JSON string.

    Accepts one positional parameter, a JSON string. It returns the Starlark
    value that the string denotes:

      - Numbers are parsed as int or float, depending on whether they contain
        a decimal point.
      - JSON objects are parsed as new unfrozen Starlark dicts.
      - JSON arrays are parsed as new unfrozen Starlark lists.

    Decoding fails if `str` is not a valid JSON string.

    Args:
      str: a JSON string to decode. Required.
    """
    _unused(str)

def _json_indent(str, *, prefix = "", indent = "\t"):
    """Pretty-prints a valid JSON encoding.

    Args:
      str: the JSON string to pretty-print. Required.
      prefix: a prefix of each new line.
      indent: a unit of indentation.

    Returns:
      The indented form of `str`.
    """
    _unused(str, prefix, indent)

def _unused(*args):  # @unused
    """Used exclusively to shut up `unused-variable` lint.

    DocTags:
      Hidden.
    """

json = struct(
    encode = _json_encode,
    decode = _json_decode,
    indent = _json_indent,
)
