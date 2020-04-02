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

"""ResultDB related supporting structs and functions."""

def _test_results_export(
    test_id_regex=None,
    contains=False,
    unexpected_only=False,
    variant=None,
  ):
  """TODO"""
  _test_results_export_ctor = __native__.genstruct('resultdb.test_results_export')
  return _test_results_export_ctor(
      test_id_regex=test_id_regex,
      contains=contains,
      unexpected_only=unexpected_only,
      variant=variant)


def _bigquery_export(
    project=None,
    dataset=None,
    table=None,
    test_results_export=None,
  ):
  """TODO"""
  _bigquery_export_ctor = __native__.genstruct('resultdb.bigquery_export')
  return _bigquery_export_ctor(
      project=project,
      dataset=dataset,
      table=table,
      test_results_export=test_results_export)


def _settings(
    enabled=False,
    bigquery_exports=None,
  ):
  """TODO"""
  _settings_ctor = __native__.genstruct('resultdb.settings')
  return _settings_ctor(
      enabled=enabled,
      bigquery_exports=bigquery_exports)

def _validate_settings(s):
  """TODO"""
  # TODO If enabled, all biqquery_exports need to be valid
  return s

def _validate_bigquery_export(be):
  """TODO"""
  # TODO project, dataset and table must all be set.
  # any test_results_export must be valid.
  return be

def _validate_test_results_export(tre):
  """TODO"""
  # TODO test_id_regex if set must be a regex.
  # expectancy must be either all or unexpected
  # variant criteria must be equals or contains
  # variant value must be a map from string to string
  return tre

resultdb = struct(
  settings = _settings,
  bigquery_export = _bigquery_export,
  test_results_export = _test_results_export,
  validate_settings = _validate_settings,
  validate_bigquery_export = _validate_bigquery_export,
  validate_test_results_export = _validate_test_results_export,
)
