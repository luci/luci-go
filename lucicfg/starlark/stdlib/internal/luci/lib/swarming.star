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

"""Swarming related supporting structs and functions."""

load('@stdlib//internal/luci/lib/validate.star', 'validate')
load('@stdlib//internal/time.star', 'time')


# See https://chromium.googlesource.com/infra/luci/luci-py/+/master/appengine/swarming/server/config.py
_DIMENSION_VALUE_LENGTH = 256
_DIMENSION_KEY_RE = r'^[a-zA-Z\-\_\.]{1,64}$'


# See https://chromium.googlesource.com/infra/infra/+/master/appengine/cr-buildbucket/swarming/swarmingcfg.py
#
# Except we drop {1,4096} because Go regexp engine refuses to use such high
# repetition numbers (and 4K for a name is effectively unbounded anyway). Let
# the server deal with such crazy values itself.
_CACHE_NAME_RE = r'^[a-z0-9_]+$'


# A constructor for swarming.cache structs.
#
# See swarming.cache(...) function below for all details.
#
# Fields:
#   name: str, name of the cache.
#   path: str, where it is mounted to.
#   wait_mins: int or None, how long to wait for a bot with warm cache.
_cache_ctor = genstruct('swarming.cache')


# A constructor for swarming.dimension structs.
#
# See swarming.dimension(...) function below for all details.
#
# Fields:
#   value: str, value of the dimension.
#   expiration_mins: int or None, when it expires.
_dimension_ctor = genstruct('swarming.dimension')


def _cache(name, path=None, wait_for_warm_cache=None):
  """A named cache entry: a cache directory persisted on a bot.

  If a build requested a cache, a cache directory is available on build
  startup. If the cache was present on the bot, the directory contains files
  from the previous run.

  The build can read/write to the cache directory while it runs. After build
  completes, the cache directory is persisted. Next time another build requests
  the same cache and runs on the same bot, if the cache wasn't evicted, the
  files will still be there.

  One bot can keep multiple caches at the same time and one build can request
  multiple different caches.

  A cache is identified by its name and mapped to a path. In recipes-based
  builds, the path is relative to api.paths['cache'] dir.

  For example, a cache swarming.cache('foo', path='bar') maps foo to bar and
  the cache dir is available at

      my_cache = api.path['cache'].join('bar')

  If the bot is running out of space, caches are evicted in LRU manner.

  Renaming a cache is equivalent to clearing it from the builder perspective.
  The files will still be there, but eventually will be purged by GC.

  Additionally, Buildbucket always implicitly declares a cache (called
  builder cache): swarming.cache(hash('<bucket>/<builder>''), path='builder')

  This means that any LUCI builder has a "personal disk space" on the bot.
  Builder cache is often a good start before customizing caching. In recipes, it
  is available at api.path['cache'].join('builder').

  In order to share the builder cache directory among multiple builders, it
  can be explicitly defined as a named cache with path "builder" in multiple
  builder rules.

  For example, if builders 'a' and 'b' both declare they use named cache
  swarming.cache('my_shared_cache', path='builder'), and an 'a' build ran on
  a bot and left some files in the builder cache, then when a 'b' build runs on
  the same bot, the same files will be available in its builder cache.

  If the pool of swarming bots is shared among multiple LUCI projects and
  projects use same cache name, the cache will be shared across projects.
  To avoid affecting and being affected by other projects, prefix the cache
  name with something project-specific, e.g. "v8-".

  Args:
    name: identifier of the cache.
    path: relative path where the cache in mapped into, default to same value
        as 'name'. Must use POSIX format (forward slashes). In most cases, it
        does not need slashes at all. Must be unique in the given builder.
    wait_for_warm_cache: how long to wait (in minutes precision) for a bot
        with a warm cache to pick up the task before falling back to a bot with
        a cold (non-existent) cache. By default (if unset or zero) no preference
        will be chosen for a bot with this or without this cache, and a bot
        without this cache may be chosen instead. If no bot has this cache warm,
        the task will skip this waiting and will immediately fallback to a cold
        cache request.

  Returns:
    swarming.cache struct.
  """
  name = validate.string('name', name, regexp=_CACHE_NAME_RE)
  path = validate.string('path', path, default=name, required=False)

  # See also _validate_relative_path in appengine/cr-buildbucket/swarming/swarmingcfg.py.
  if not path:
    fail('bad cache %r: path must not be empty' % (name,))
  if '\\' in path:
    fail('bad cache %r: path %r must not contain "\\", for Windows sake' % (name, path))
  if '..' in path.split('/'):
    fail('bad cache %r: path %r must not contain ".."' % (name, path))
  if path.startswith('/'):
    fail('bad cache %r: path %r must not start with "/"' % (name, path))

  return _cache_ctor(
      name = name,
      path = path,
      wait_mins = validate.duration(
          'wait_for_warm_cache',
          wait_for_warm_cache,
          precision=time.minute,
          min=time.minute,
          required=False,
      ),
  )


def _dimension(value, expiration=None):
  """A value of some Swarming dimension, annotated with its expiration time.

  Intended to be used as a value in 'dimensions' dict when using dimensions
  that expire:

      dimensions = {
          ...
          'device': swarming.dimension('preferred', expiration=5*time.minute),
          ...
      }

  Args:
    value: string value of the dimension.
    expiration: how long to wait (in minutes precision) for a bot with this
        dimension, or None to wait until the overall build expiration deadline.

  Returns:
    swarming.dimension struct.
  """
  # See also 'validate_dimension_value' in appengine/swarming/server/config.py.
  val = validate.string('value', value)
  if not val:
    fail('bad dimension value: must not be empty')
  if len(val) > _DIMENSION_VALUE_LENGTH:
    fail('bad dimension value %r: must be at most %d chars' % (val, _DIMENSION_VALUE_LENGTH))
  if val != val.strip():
    fail('bad dimension value %r: must not have leading or trailing whitespace' % (val,))
  return _dimension_ctor(
      value = val,
      expiration_mins = validate.duration(
          'expiration',
          expiration,
          precision=time.minute,
          min=time.minute,
          required=False,
      ),
  )


def _validate_caches(attr, caches):
  """Validates a list of caches.

  Ensures each entry is swarming.cache struct, and no two entries use same
  name or path.

  Args:
    attr: field name with caches, for error messages.
    caches: a list of swarming.cache(...) entries to validate.

  Returns:
    Validate list of caches (may be an empty list, never None).
  """
  per_name = {}
  per_path = {}
  caches = validate.list(attr, caches)
  for c in caches:
    validate.struct(attr, c, _cache_ctor)
    if c.name in per_name:
      fail('bad "caches": caches %s and %s have same name' % (c, per_name[c.name]))
    if c.path in per_path:
      fail('bad "caches": caches %s and %s use same path' % (c, per_path[c.path]))
    per_name[c.name] = c
    per_path[c.path] = c
  return caches


def _validate_dimensions(attr, dims):
  """Validates a dict with dimensions.

  Args:
    attr: field name with dimensions, for error messages.
    dims: a dict {string: string|swarming.dimension}.

  Returns:
    Validated and normalized dict with dimensions {string: swarming.dimension}.
  """
  out = {}
  for k, v in validate.str_dict(attr, dims).items():
    # See also 'validate_dimension_key' in appengine/swarming/server/config.py.
    validate.string(attr, k, regexp=_DIMENSION_KEY_RE)
    if type(v) == 'string':
      out[k] = _dimension(v)
    else:
      out[k] = validate.struct(attr, v, _dimension_ctor)
  return out


def _validate_tags(attr, tags):
  """Validates a list of "k:v" pairs with Swarming tags.

  Args:
    attr: field name with tags, for error messages.
    tags: a list of tags to validate.

  Returns:
    Validated list of tags in same order, with duplicates removed.
  """
  out = set()  # note: in starlark sets/dicts remember the order
  for t in validate.list(attr, tags):
    validate.string(attr, t, regexp=r'.+\:.+')
    out = out.union([t])
  return list(out)


# Public API exposed to end-users and to other LUCI modules.
swarming = struct(
    cache = _cache,
    dimension = _dimension,
)


# Additional internal API used by other LUCI modules.
swarmingimpl = struct(
    validate_caches = _validate_caches,
    validate_dimensions = _validate_dimensions,
    validate_tags = _validate_tags,
)
