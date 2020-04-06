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

load('@stdlib//internal/luci/proto.star',
    'buildbucket_pb',
    'predicate_pb',
    'resultdb_pb',
)


def _settings(*, enable=False, bigquery_exports=None):
  """Specifies how buildbucket should sync a builder's builds with ResultDB.

  Args:
    enable: boolean, whether to enable ResultDB:Buildbucket integration.
    bigquery_exports: list of resultdb_pb.BigQueryExport() protos,
        configurations for exporting specific subsets of test results to a
        designated BigQuery table.

  Returns:
    A populated buildbucket_pb.Builder.ResultDB() proto.
  """
  ret = buildbucket_pb.Builder.ResultDB(
      enable = validate.bool('enable', enable, default=False, required=False),
  )
  if bigquery_exports:
    ret.bq_exports.extend(bigquery_exports)
  return ret


def _bigquery_export(
      *,
      bq_table = None,
      test_results_predicate = None
  ):
  """Represents a mapping between a subset of test results and a BigQuery table
  to export them to.

  Args:
    bq_table: string of the form `<project>.<dataset>.<table>`
        where the parts respresent the BigQuery-enabled gcp project, dataset
        and table to export results.
    test_results_predicate: A predicate_pb.TestResultPredicate() proto,
        specifies the subset of test results to export to the above table.
        if unspecified, matches all test results.

  Returns:
    A populated resultdb_pb.BigQueryExport() proto.
  """
  bq_table_parts = validate.string('bq_table', bq_table).split('.')

  if not bq_table_parts or len(bq_table_parts) != 3 or not all(bq_table_parts):
    fail("bad 'bq_table': have %s, want %s"
         % (bq_table, '<project>.<dataset>.<table>'))

  project, dataset, table = bq_table_parts
  ret = resultdb_pb.BigQueryExport(
      project = project,
      dataset = dataset,
      table = table,
  )
  if test_results_predicate:
    ret.test_results.predicate = test_results_predicate
  return ret


def _test_results_predicate(
      *,
      test_id_regexp = None,
      variant_contains = False,
      unexpected_only = False,
      variant = None
  ):
  """Represents a predicate of test results.

  Args:
    test_id_regexp: string, regular expression that a test result must fully
        match to be considered covered by this definition.
    variant: string dict, defines the test variant to match.
        E.g. {"test_suite": "not_site_per_process_webkit_layout_tests"}
    variant_contains: bool, if true the variant parameter above will cause a
        match if it's a subset of the test's variant, otherwise it will only
        match if it's exactly equal.
    unexpected_only: bool, if true only export test results of test variants
        that had unexpected results.

  Returns:
    A populated predicate_pb.TestResultPredicate() proto.
  """
  ret = predicate_pb.TestResultPredicate(
      test_id_regexp = validate.string(
          'test_id_regexp', test_id_regexp, required=False),
  )

  unexpected_only = validate.bool('unexpected_only', unexpected_only,
                                  default=False, required=False)
  if unexpected_only:
    ret.expectancy = predicate_pb.TestResultPredicate.VARIANTS_WITH_UNEXPECTED_RESULTS
  else:
    ret.expectancy = predicate_pb.TestResultPredicate.ALL

  variant_contains = validate.bool('variant_contains', variant_contains,
                                   default=True, required=False)
  variant = validate.str_dict('variant', variant, required=False)
  if variant_contains:
    ret.variant.contains = {'def': variant}
  else:
    ret.variant.equals = {'def': variant}

  return ret


def _validate_settings(settings):
  return validate.type(
      'settings',
      settings,
      buildbucket_pb.Builder.ResultDB(),
      required=False,
  )


resultdb = struct(
    settings = _settings,
    bigquery_export = _bigquery_export,
    test_results_predicate = _test_results_predicate,
    validate_settings = _validate_settings,
)
