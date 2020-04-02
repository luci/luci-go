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

load('@stdlib//internal/validate.star', 'validate')


# TODO
_settings_ctor = __native__.genstruct('resultdb.settings')


# TODO
_bigquery_export_ctor = __native__.genstruct('resultdb.bigquery_export')


# TODO
_test_results_export_ctor = __native__.genstruct('resultdb.test_results_export')


def _settings(enable=False, bigquery_exports=None):
  """TODO"""
  return _settings_ctor(
      name = 'resultdb.settings',
      enable = validate.bool('enable', enable, default=False, required=False),
      bigquery_exports = [
          validate.struct(
	      'bigquery_export', bqe, _bigquery_export_ctor, required=False)
          for bqe in validate.list('bigquery_exports', bigquery_exports)
      ],
  )


def _bigquery_export(
    project=None,
    dataset=None,
    table=None,
    test_results_export=None):
  """TODO"""
  return _bigquery_export_ctor(
      project = validate.string('project', project),
      dataset = validate.string('dataset', dataset),
      table = validate.string('table', table),
      test_results_export = validate.struct(
          'test_results_export', test_results_export, _test_results_export_ctor,
          required=False),
  )


def _test_results_export(
    test_id_regexp = None,
    contains = False,
    unexpected_only = False,
    variant = None):
  """TODO"""
  return _test_results_export_ctor(
      test_id_regexp = validate.string(
          'test_id_regexp', test_id_regexp, required=False),
      unexpected_only = validate.bool(
          'unexpected_only', unexpected_only, default=False, required=False),
      contains = validate.bool(
          'contains', contains, default=True, required=False),
      variant = validate.str_dict('variant', variant, required=False),
  )


def _validate_settings(settings):
  """TODO"""
  return validate.struct('settings', settings, _settings_ctor, required=False)


resultdb = struct(
  settings = _settings,
  bigquery_export = _bigquery_export,
  test_results_export = _test_results_export,
  validate_settings = _validate_settings,
)
