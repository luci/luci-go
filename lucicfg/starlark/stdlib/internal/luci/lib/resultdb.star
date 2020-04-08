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


def _settings(*, enable=False, bq_exports=None):
  """Specifies how buildbucket should integrate with ResultDB.

  Args:
    enable: boolean, whether to enable ResultDB:Buildbucket integration.
    bq_exports: list of resultdb_pb.BigQueryExport() protos, configurations for
        exporting specific subsets of test results to a designated BigQuery
        table, use resultdb.export_test_results(...) to create these.

  Returns:
    A populated buildbucket_pb.Builder.ResultDB() proto.
  """
  return buildbucket_pb.Builder.ResultDB(
      enable = validate.bool('enable', enable, default=False, required=False),
      bq_exports = bq_exports or [],
  )


def _export_test_results(
      *,
      bq_table = None,
      predicate = None
  ):
  """Represents a mapping between a subset of test results and a BigQuery table
  to export them to.

  Args:
    bq_table: string of the form `<project>.<dataset>.<table>`
        where the parts respresent the BigQuery-enabled gcp project, dataset
        and table to export results.
    predicate: A predicate_pb.TestResultPredicate() proto. If given, specifies
        the subset of test results to export to the above table, instead of all.
        Use resultdb.test_result_predicate(...) to generate this, if needed.

  Returns:
    A populated resultdb_pb.BigQueryExport() proto.
  """
  project, dataset, table = validate.string(
      'bq_table', bq_table, regexp=r'^([^.]+)\.([^.]+)\.([^.]+)$').split('.')

  return resultdb_pb.BigQueryExport(
      project = project,
      dataset = dataset,
      table = table,
      test_results = resultdb_pb.BigQueryExport.TestResults(
          predicate = validate.type(
              'predicate', predicate,
              predicate_pb.TestResultPredicate(),
              required=False,
          ),
      ),
  )


def _test_result_predicate(
      *,
      test_id_regexp = None,
      variant = None,
      variant_contains = False,
      unexpected_only = False
  ):
  """Represents a predicate of test results.

  Args:
    test_id_regexp: string, regular expression that a test result must fully
        match to be considered covered by this definition.
    variant: string dict, defines the test variant to match.
        E.g. `{"test_suite": "not_site_per_process_webkit_layout_tests"}`
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
                                   default=False, required=False)
  variant = validate.str_dict('variant', variant, required=False)
  if variant:
    if variant_contains:
      ret.variant.contains = {'def': variant}
    else:
      ret.variant.equals = {'def': variant}

  return ret


def _validate_settings(settings):
  """Validates the type of a ResultDB settings proto.

  Args:
    settings: A proto such as the one returned by resultdb.settings(...).

  Returns:
    A validated proto, if it's the correct type.
  """
  return validate.type(
      'settings',
      settings,
      buildbucket_pb.Builder.ResultDB(),
      required=False,
  )


resultdb = struct(
    settings = _settings,
    export_test_results = _export_test_results,
    test_result_predicate = _test_result_predicate,
    validate_settings = _validate_settings,
)
