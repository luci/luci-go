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


def _config(
      config_service_host=None,
      config_set=None,
      config_dir=None,
      tracked_files=None,
      fail_on_warnings=None,
  ):
  """Sets one or more parameters for the `lucicfg` itself.

  These parameters do not affect semantic meaning of generated configs, but
  influence how they are generated and validated.

  Each parameter has a corresponding command line flag. If the flag is present,
  it overrides the value set via `meta.config` (if any). For example, the flag
  `-config-service-host <value>` overrides whatever was set via
  `meta.config(config_service_host=...)`.

  `meta.config` is allowed to be called multiple times. The most recently set
  value is used in the end, so think of `meta.config(var=...)` just as assigning
  to a variable.

  Args:
    config_service_host: a hostname of a LUCI Config Service to send validation
        requests to. Default is whatever is hardcoded in `lucicfg` binary,
        usually `luci-config.appspot.com`.
    config_set: name of the config set in LUCI Config Service to use for
        validation. Default is `projects/<name>` where `<name>` is taken from
        core.project(...) rule. If there's no such rule, the default is "",
        meaning the generated config will not be validated via LUCI Config
        Service.
    config_dir: a directory to place generated configs into, relative to the
        directory that contains the entry point \*.star file. `..` is allowed.
        If set via `-config-dir` command line flag, it is relative to the
        current working directory. Will be created if absent. If `-`, the
        configs are just printed to stdout in a format useful for debugging.
        Default is "generated".
    tracked_files: a list of glob patterns that define a subset of files under
        `config_dir` that are considered generated. This is important if some
        generated file disappears from `lucicfg` output: it must be deleted from
        the disk as well. To do this, `lucicfg` needs to know what files are
        safe to delete. Each entry is either `<glob pattern>` (a "positive"
        glob) or `!<glob pattern>` (a "negative" glob). A file under
        `config_dir` (or any of its subdirectories) is considered tracked if it
        matches any of the positive globs and none of the negative globs. For
        example, `tracked_files` for prod and dev projects co-hosted in the same
        directory may look like `['*.cfg', '!*-dev.cfg']` for prod and
        `['*-dev.cfg']` for dev. If `tracked_files` is empty (default), lucicfg
        will never delete any files. In this case it is responsibility of the
        caller to make sure no stale output remains.
    fail_on_warnings: if set to True treat validation warnings as errors.
        Default is False (i.e. warnings do to cause the validation to fail). If
        set to True via `meta.config` and you want to override it to False via
        command line flags use `-fail-on-warnings=false`.
  """
  if config_service_host != None:
    __native__.set_meta('config_service_host', config_service_host)
  if config_set != None:
    __native__.set_meta('config_set', config_set)
  if config_dir != None:
    __native__.set_meta('config_dir', config_dir)
  if tracked_files != None:
    __native__.set_meta('tracked_files', tracked_files)
  if fail_on_warnings != None:
    __native__.set_meta('fail_on_warnings', fail_on_warnings)


def _version():
  """Returns a triple with lucicfg version: `(major, minor, revision)`."""
  return __native__.version()


# Public API.

meta = struct(
    config = _config,
    version = _version,
)
