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

"""Generic value validators."""

load('@stdlib//internal/re.star', 're')
load('@stdlib//internal/time.star', 'time')


def _string(attr, val, regexp=None, default=None):
  """Validates that the value is a string and returns it.

  Args:
    attr: field name with this value, for error messages.
    val: a value to validate.
    regexp: a regular expression to check 'val' against.
    default: a value to use if 'val' is None, or None if the value is required.

  Returns:
    The validated string.
  """
  if val == None:
    if default == None:
      fail('bad %r: missing' % attr)
    val = default

  if type(val) != 'string':
    fail('bad %r: got %s, want string' % (attr, type(val)))

  if regexp and not re.submatches(regexp, val):
    fail('bad %r: %r should match %r' % (attr, val, regexp))

  return val


def _duration(attr, val, precision=time.second, min=time.zero, max=None, default=None):
  """Validates that the value is a duration specified at the given precision.

  For example, if 'precision' is time.second, will validate that the given
  duration has a whole number of seconds and will return them (as integer).
  Fails if truncating the duration to the given precision loses information.

  Args:
    attr: field name with this value, for error messages.
    val: a value to validate.
    precision: a time unit to divide 'val' by to get the output.
    min: minimal allowed duration (inclusive) or None for unbounded.
    max: maximal allowed duration (inclusive) or None for unbounded.
    default: a value to use if 'val' is None, or None if the value is required.

  Returns:
    An integer: a whole number of 'precision' units in the given duration value.
  """
  if val == None:
    if default == None:
      fail('bad %r: missing' % attr)
    val = default

  if type(val) != 'duration':
    fail('bad %r: got %s, want duration' % (attr, type(val)))

  if min != None and val < min:
    fail('bad %r: %s should be >= %s' % (attr, val, min))
  if max != None and val > max:
    fail('bad %r: %s should be <= %s' % (attr, val, max))

  res = val / precision
  if res * precision != val:
    fail((
        'bad %r: losing precision when truncating %s to %s units, ' +
        'use time.truncate(...) to acknowledge') % (attr, val, precision))

  return res


def _list(attr, val, required=False):
  """Validates that the value is a list and returns it.

  None is treated as an empty list.

  Args:
    attr: field name with this value, for error messages.
    val: a value to validate.
    required: if True, reject empty lists.

  Returns:
    The validated list.
  """
  if val == None:
    val = []

  if type(val) != 'list':
    fail('bad %r: got %s, want list' % (attr, type(val)))

  if required and not val:
    fail('bad %r: missing' % attr)

  return val


def _struct(attr, val, sym, default=None):
  """Validates that the value is a struct of the given flavor and returns it.

  Args:
    attr: field name with this value, for error messages.
    val: a value to validate.
    sym: a name of the constructor that produced the struct.
    default: a value to use if 'val' is None, or None if the value is required.

  Returns:
    The validated struct.
  """
  if val == None:
    if default == None:
      fail('bad %r: missing' % attr)
    val = default

  tp = ctor(val) or type(val)  # ctor(...) return None for non-structs
  if tp != sym:
    fail('bad %r: got %s, want %s' % (attr, tp, sym))

  return val


validate = struct(
    string = _string,
    duration = _duration,
    list = _list,
    struct = _struct,
)
