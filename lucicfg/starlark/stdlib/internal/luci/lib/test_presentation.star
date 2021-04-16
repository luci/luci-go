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

"""Test presentation related supporting structs and functions."""

load("@stdlib//internal/validate.star", "validate")

# A struct returned by test_presentation.config.
#
# See test_presentation.config function for all details.
#
# Fields:
#   column_keys: list of string, a list of keys that will be rendered as
#     'columns'.
#   grouping_keys: list of string, a list of keys that will be used for grouping
#     tests.
_test_presentation_config_ctor = __native__.genstruct("test_presentation.config")

def _config(*, column_keys = None, grouping_keys = None):
    """Specifies how test should be rendered.

    Args:
      column_keys: list of string, a list of keys that will be rendered as
        'columns'. status is always the first column and name is always the last
        column (you don't need to specify them). A key must be one of the
        following:
          1. 'v.{variant_key}': variant.def[variant_key] of the test variant
            (e.g. v.gpu).
        If None, defaults to [].
      grouping_keys: list of string, a list of keys that will be used for
        grouping tests. A key must be one of the following:
          1. 'status': status of the test variant.
          2. 'name': name of the test variant.
          3. 'v.{variant_key}': variant.def[variant_key] of the test variant
            (e.g. v.gpu).
        If None, defaults to ['status'].
        Caveat: test variants with only expected results are not affected by
          this setting and are always in their own group.

    Returns:
      test_presentation.config struct with fields `column_keys` and
      `grouping_keys`.
    """
    column_keys = validate.str_list("column_keys", column_keys)
    for key in column_keys:
        if not key.startswith("v."):
            fail("invalid column key: %r should be a variant key with 'v.' prefix" % key)

    grouping_keys = validate.str_list("grouping_keys", grouping_keys) or ["status"]
    for key in grouping_keys:
        if key not in ["status", "name"] and not key.startswith("v."):
            fail("invalid grouping key: %r should be 'status', 'name', or a variant key with 'v.' prefix" % key)

    return _test_presentation_config_ctor(
        column_keys = column_keys,
        grouping_keys = grouping_keys,
    )

def _validate_config(attr, config):
    """Validates a test presentation config.

    Args:
      attr: field name with caches, for error messages. Required.
      config: a test_presentation.config to validate.

    Returns:
      A validated test_presentation.config.
    """
    return validate.struct(attr, config, _test_presentation_config_ctor, default = _config(), required = False)

test_presentation = struct(
    config = _config,
    validate_config = _validate_config,
)
