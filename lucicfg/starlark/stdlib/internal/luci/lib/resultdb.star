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

load("@stdlib//internal/validate.star", "validate")
load(
    "@stdlib//internal/luci/proto.star",
    "buildbucket_pb",
    "predicate_pb",
    "resultdb_pb",
)

def _settings(*, enable = False, bq_exports = None, history_options = None):
    """Specifies how Buildbucket should integrate with ResultDB.

    Args:
      enable: boolean, whether to enable ResultDB:Buildbucket integration.
      bq_exports: list of resultdb_pb.BigQueryExport() protos, configurations
        for exporting specific subsets of test results to a designated BigQuery
        table, use resultdb.export_test_results(...) to create these.
      history_options: Configuration for indexing test results from this
        builder's builds for history queries, use resultdb.history_options(...)
        to create this value.

    Returns:
      A populated buildbucket_pb.BuilderConfig.ResultDB() proto.
    """
    return buildbucket_pb.BuilderConfig.ResultDB(
        enable = validate.bool("enable", enable, default = False, required = False),
        bq_exports = bq_exports or [],
        history_options = history_options,
    )

def _history_options(*, by_timestamp = False):
    """Defines a history indexing configuration.

    Args:
      by_timestamp: bool, indicates whether the build's test results will be
        indexed by their creation timestamp for the purposes of retrieving the
        history of a given set of tests/variants.

    Returns:
      A populated resultdb_pb.HistoryOptions() proto.
    """
    return resultdb_pb.HistoryOptions(
        use_invocation_timestamp = by_timestamp,
    )

def _bq_export(bq_table = None):
    if type(bq_table) == "tuple":
      if len(bq_table) != 3:
        fail("Expected tuple of length 3, got %s" % (bq_table,))
      project, dataset, table = bq_table
    elif type(bq_table) == "string":
      project, dataset, table = validate.string(
          "bq_table",
          bq_table,
          regexp = r"^([^.]+)\.([^.]+)\.([^.]+)$",
      ).split(".")
    else:
      fail("Unsupported bq_table type %s" % type(bq_table))

    return resultdb_pb.BigQueryExport(
        project = project,
        dataset = dataset,
        table = table,
    )

def _export_test_results(
        *,
        bq_table = None,
        predicate = None):
    """Defines a mapping between a test results and a BigQuery table for them.

    Args:
      bq_table: Tuple of `(project, dataset, table)`; OR a string of the form
        `<project>.<dataset>.<table>` where the parts represent the
        BigQuery-enabled gcp project, dataset and table to export results.
      predicate: A predicate_pb.TestResultPredicate() proto. If given, specifies
        the subset of test results to export to the above table, instead of all.
        Use resultdb.test_result_predicate(...) to generate this, if needed.

    Returns:
      A populated resultdb_pb.BigQueryExport() proto.
    """
    ret = _bq_export(bq_table)
    ret.test_results = resultdb_pb.BigQueryExport.TestResults(
        predicate = validate.type(
            "predicate",
            predicate,
            predicate_pb.TestResultPredicate(),
            required = False,
        ),
    )
    return ret

def _test_result_predicate(
        *,
        test_id_regexp = None,
        variant = None,
        variant_contains = False,
        unexpected_only = False):
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
            "test_id_regexp",
            test_id_regexp,
            required = False,
        ),
    )

    unexpected_only = validate.bool(
        "unexpected_only",
        unexpected_only,
        default = False,
        required = False,
    )
    if unexpected_only:
        ret.expectancy = predicate_pb.TestResultPredicate.VARIANTS_WITH_UNEXPECTED_RESULTS
    else:
        ret.expectancy = predicate_pb.TestResultPredicate.ALL

    variant_contains = validate.bool(
        "variant_contains",
        variant_contains,
        default = False,
        required = False,
    )
    variant = validate.str_dict("variant", variant, required = False)
    if variant:
        if variant_contains:
            ret.variant.contains = {"def": variant}
        else:
            ret.variant.equals = {"def": variant}

    return ret

def _export_text_artifacts(
        *,
        bq_table = None,
        predicate = None):
    """Defines a mapping between text artifacts and a BigQuery table for them.

    Args:
      bq_table: string of the form `<project>.<dataset>.<table>`
        where the parts respresent the BigQuery-enabled gcp project, dataset and
        table to export results.
      predicate: A predicate_pb.ArtifactPredicate() proto. If given, specifies
        the subset of text artifacts to export to the above table, instead of all.
        Use resultdb.artifact_predicate(...) to generate this, if needed.

    Returns:
      A populated resultdb_pb.BigQueryExport() proto.
    """
    ret = _bq_export(bq_table)
    ret.text_artifacts = resultdb_pb.BigQueryExport.TextArtifacts(
        predicate = validate.type(
            "predicate",
            predicate,
            predicate_pb.ArtifactPredicate(),
            required = False,
        ),
    )
    return ret

def _artifact_predicate(
        *,
        test_result_predicate = None,
        included_invocations = None,
        test_results = None,
        content_type_regexp = None,
        artifact_id_regexp = None):
    """Represents a predicate of text artifacts.

    Args:
      test_result_predicate: predicate_pb.TestResultPredicate(), a predicate of
        test results.
      included_invocations: bool, if true, invocation level artifacts are
        included.
      test_results: bool, if true, test result level artifacts are included.
      content_type_regexp: string, an artifact must have a content type matching
        this regular expression entirely, i.e. the expression is implicitly
        wrapped with ^ and $.
      artifact_id_regexp: string, an artifact must have an ID matching this
        regular expression entirely, i.e. the expression is implicitly wrapped
        with ^ and $.

    Returns:
      A populated predicate_pb.ArtifactPredicate() proto.
    """
    ret = predicate_pb.ArtifactPredicate(
        test_result_predicate = validate.type(
            "test_result_predicate",
            test_result_predicate,
            predicate_pb.TestResultPredicate(),
            required = False,
        ),
        content_type_regexp = validate.string(
            "content_type_regexp",
            content_type_regexp,
            required = False,
        ),
        artifact_id_regexp = validate.string(
            "artifact_id_regexp",
            artifact_id_regexp,
            required = False,
        ),
    )

    included_invocations = validate.bool(
        "included_invocations",
        included_invocations,
        default = None,
        required = False,
    )
    if included_invocations != None:
        ret.follow_edges.included_invocations = included_invocations

    test_results = validate.bool(
        "test_results",
        test_results,
        default = None,
        required = False,
    )
    if test_results != None:
        ret.follow_edges.test_results = test_results

    return ret

def _validate_settings(attr, settings):
    """Validates the type of a ResultDB settings proto.

    Args:
      attr: field name with settings, for error messages. Required.
      settings: A proto such as the one returned by resultdb.settings(...).

    Returns:
      A validated proto, if it's the correct type.
    """
    return validate.type(
        attr,
        settings,
        buildbucket_pb.BuilderConfig.ResultDB(),
        required = False,
    )

# A struct returned by resultdb.test_presentation.
#
# See resultdb.test_presentation function for all details.
#
# Fields:
#   column_keys: list of string keys that will be rendered as 'columns'.
#   grouping_keys: list of string keys that will be used for grouping tests.
_test_presentation_config_ctor = __native__.genstruct("test_presentation.config")

def _test_presentation(*, column_keys = None, grouping_keys = None):
    """Specifies how test should be rendered.

    Args:
      column_keys: list of string keys that will be rendered as 'columns'.
        status is always the first column and name is always the last column
        (you don't need to specify them). A key must be one of the following:
          1. 'v.{variant_key}': variant.def[variant_key] of the test variant
            (e.g. v.gpu).
        If None, defaults to [].
      grouping_keys: list of string keys that will be used for grouping tests.
        A key must be one of the following:
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

def _validate_test_presentation(attr, config, required = False):
    """Validates a test presentation config.

    Args:
      attr: field name with caches, for error messages. Required.
      config: a test_presentation.config to validate.
      required: if False, allow 'config' to be None, return None in this case.

    Returns:
      A validated test_presentation.config.
    """
    return validate.struct(attr, config, _test_presentation_config_ctor, required = required)

def _test_presentation_to_dict(config):
    """Converts a test presentation config to a dictionary.

    Args:
      config: a test_presentation.config to be converted to a dictionary.

    Returns:
      A dictionary representing the test presentation config.
    """

    return {
        "column_keys": config.column_keys,
        "grouping_keys": config.grouping_keys,
    }

resultdb = struct(
    settings = _settings,
    export_test_results = _export_test_results,
    test_result_predicate = _test_result_predicate,
    validate_settings = _validate_settings,
    history_options = _history_options,
    export_text_artifacts = _export_text_artifacts,
    artifact_predicate = _artifact_predicate,
    test_presentation = _test_presentation,
    validate_test_presentation = _validate_test_presentation,
)

resultdbimpl = struct(
    test_presentation_to_dict = _test_presentation_to_dict,
)
