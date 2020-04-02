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

"""ResultDB integration settings."""

load('@stdlib//internal/validate.star', 'validate')


# A struct returned by resultdb.settings(...)
#
# See resultdb.settings(...) function for all details.
#
# Fields:
#   enable: boolean, whether buildbucket should sync a builder's builds with
#       resultdb.
#   bigquery_exports: list of resultdb.bigquery_export(...), configurations
#       for exporting specific subsets of test results to a designated bigquery
#       tables.
_settings_ctor = __native__.genstruct('resultdb.settings')


# A struct returned by resultdb.bigquery_export(...)
#
# See resultdb.bigquery_export(...) function for all details.
#
# Fields:
#   project: string, the BigQuery-enabled gcp project.
#   dataset: string, the dataset within the above project.
#   table: string, the name of the table in the above dataset to export results
#       into.
#   test_results_export: resultdb.test_results_export(...), specifies a subset
#       of test results to export.
#
_bigquery_export_ctor = __native__.genstruct('resultdb.bigquery_export')


# A struct returned by resultdb.test_results_export(...)
#
# See resultdb.test_results_export(...) function for all details.
#
# Fields:
#   test_id_regexp: string, regular expression that a test result must fully
#       match to be considered covered by this definition.
#   variant: string dict, defines the test variant to match.
#   contains: bool, if false the test's variant must equal the variant above
#       exactly, otherwise it may be a superset of it.
#   unexpected_only: bool, if true only export results that are unexpected,
#       otherwise match all results.
_test_results_export_ctor = __native__.genstruct('resultdb.test_results_export')


def _settings(enable=False, bigquery_exports=None):
  """Specifies how buildbucket should sync a builder's builds with ResultDB.

  Args:
    enable: boolean, whether buildbucket should sync a builder's builds with
        resultdb.
    bigquery_exports: list of resultdb.bigquery_export(...), configurations
        for exporting specific subsets of test results to a designated bigquery
        table.
  """
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
  """Represents a mapping between a subset of test results in a given builder
  and a buildbucket table to export them to.

  Args:
    project: string, the BigQuery-enabled gcp project.
    dataset: string, the dataset within the above project.
    table: string, the name of the table in the above dataset to export results
        into.
    test_results_export: resultdb.test_results_export(...), specifies the subset
        of test results to export.
  """
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
  """Represents a subset of test results.

  The subset being defined by:
    - matching the test_id with a regular expression
    - comparing the test's variant with a parameter and
      - matching it exactly, or the latter being a subset of the former.
    - checking wether the result (passing) status matches the expectation.

  Args:
    test_id_regexp: string, regular expression that a test result must fully
        match to be considered covered by this definition.
    variant: string dict, defines the test variant to match.
    contains: bool, if true the variant parameter above will cause a match if
        it's contained in the test's variant, otherwise it will only match if
	it's exactly equal.
    unexpected_only: bool, if true only export results that are unexpected,
        otherwise match all results.
  """
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
  return validate.struct('settings', settings, _settings_ctor, required=False)


resultdb = struct(
  settings = _settings,
  bigquery_export = _bigquery_export,
  test_results_export = _test_results_export,
  validate_settings = _validate_settings,
)
