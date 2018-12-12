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


def _string(attr, val, default='', required=True):
  if val == None:
    if required:
      fail('bad %r: missing' % attr)
    return default
  if type(val) != 'string':
    fail('bad %r: got %s %r, expecting string' % (attr, type(val), val))
  return val


def _list(attr, val, default=None, required=False):
  if val != None and type(val) != 'list':
    fail('bad %r: got %s %r, expecting list' % (attr, type(val), val))
  if val:
    return val
  if required:
    fail('bad %r: missing' % attr)
  return default or []


def _struct(attr, val, sym, default=None, required=True):
  if val == None:
    if required:
      fail('bad %r: missing' % attr)
    return default
  tp = ctor(val) or type(val)  # ctor(...) return None for non-structs
  if tp != sym:
    fail('bad %r: got %s %r, expecting %s' % (attr, tp, val, sym))
  return val


validate = struct(
    string = _string,
    list = _list,
    struct = _struct,
)
