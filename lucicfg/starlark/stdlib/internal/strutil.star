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

"""Utilities for working with strings."""

load("@stdlib//internal/re.star", "re")

def _expand_int_set(s):
    """Expands string with sets into a list of strings.

    For example, given `a{1..3}b` produces `['a1b', 'a2b', 'a3b']`.

    The incoming string should have no more than one `{...}` section. If it's
    absent, the function returns the list that contains one item: the original
    string.

    The set is given as comma-separated list of terms. Each term is either
    a single non-negative integer (e.g. `9`) or a range (e.g. `1..5`). Both ends
    of the range are inclusive. Ranges where the left hand side is larger than
    the right hand side are not allowed. All elements should be listed in the
    strictly increasing order (e.g. `1,2,5..10` is fine, but `5..10,1,2` is
    not). Spaces are not allowed.

    The output integers are padded with zeros to match the width of
    corresponding terms. For ranges this works only if both sides have same
    width. For example, `01,002,03..04` will expand into `01, 002, 03, 04`.

    Use `{{` and `}}` to escape `{` and `}` respectively.

    Args:
      s: a string with the set to expand. Required.

    Returns:
      A list of strings representing the expanded set.
    """

    # Implementation is in Go, since it is simpler there, considering Starlark
    # strings aren't even iterable.
    return __native__.expand_int_set(s)

def _json_to_yaml(json):
    """Takes a JSON string and returns it as a pretty-printed YAML.

    Args:
      json: a JSON string to convert to YAML. Required.

    Returns:
      A pretty YAML string ending with `\n`.
    """
    return __native__.json_to_yaml(json)

def _to_yaml(value):
    """Serializes a value to a pretty-printed YAML string.

    Doesn't support integers that do not fit int64. Fails if the value has
    cycles.

    Args:
      value: a primitive Starlark value: a scalar, or a list/tuple/dict
        containing only primitive Starlark values. Required.

    Returns:
      A pretty YAML string ending with `\n`.
    """
    return _json_to_yaml(json.encode(value))

def _b64_encode(s):
    """Encodes a string using standard padded base64 encoding.

    Args:
      s: a string to encode. Required.

    Returns:
      A base64 string.
    """
    return __native__.b64_encode(s)

def _b64_decode(s):
    """Decodes a string encoded using standard padded base64 encoding.

    Fails if `s` is not a base64 string.

    Args:
      s: a string to decode. Required.

    Returns:
      Decoded string.
    """
    return __native__.b64_decode(s)

def _hex_encode(s):
    """Encodes a string as a sequence of hex bytes.

    Args:
      s: a string to encode. Required.

    Returns:
      A string with hexadecimal encoding.
    """
    return __native__.hex_encode(s)

def _hex_decode(s):
    """Decodes a string encoded as a sequence of hex bytes.

    Args:
      s: a string to decode. Required.

    Returns:
      Decoded string.
    """
    return __native__.hex_decode(s)

def _template(s):
    """Parses the given string as a Go text template and returns template object.

    See https://golang.org/pkg/text/template to syntax of Go text templates.

    Args:
      s: a string to parse as a template. Required.

    Returns:
      An object with `render(**kwargs)` method. It takes some kwargs with
      elementary types (strings, numbers, list and dicts) and uses them as
      inputs to the template, returning rendered template as a string.
    """
    return __native__.template(s)

def _join_path(base, rel, allow_dots = False):
    """Joins two slash-separated paths together, normalizing the result.

    Args:
      base: a string with the base path.
      rel: a string with the path to append to it.
      allow_dots: if True, allow the resulting path to start with `..`.

    Returns:
      The joined path.
    """
    res, err = __native__.clean_relative_path(base, rel, allow_dots)
    if err:
        fail(err)
    return res

def _parse_version(ver):
    """Parses `major.minor.revision` version string.

    Empty version components are assumed to be zeroes, e.g. `1.1` is the same
    as `1.1.0`. Extra components are ignored, e.g. `1.2.3.4` is the same as
    `1.2.3`. All components must be positive integers (fails otherwise), in
    particular versions like e.g. `1.2.3-rc1` aren't accepted.

    Args:
      ver: a version string to parse. Required.

    Returns:
      A triple of ints `(major, minor, revision)`.
    """
    if ver == None:
        fail("a version string is required")
    if type(ver) != "string":
        fail("bad version: got %s, want string" % type(ver))
    if not re.submatches(r"^\d+(\.\d+)*$", ver):
        fail("bad version string: should be `major.minor.revision`, got %r" % ver)
    val = [int(x) for x in ver.split(".")][:3]
    if len(val) < 3:
        val += [0] * (3 - len(val))
    return tuple(val)

strutil = struct(
    expand_int_set = _expand_int_set,
    json_to_yaml = _json_to_yaml,
    to_yaml = _to_yaml,
    b64_encode = _b64_encode,
    b64_decode = _b64_decode,
    hex_encode = _hex_encode,
    hex_decode = _hex_decode,
    template = _template,
    join_path = _join_path,
    parse_version = _parse_version,
)
