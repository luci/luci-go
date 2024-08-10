# Copyright 2024 The LUCI Authors.
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

"""Helper library for setting custom metrics."""

load("@stdlib//internal/validate.star", "validate")

# A struct returned by buildbucket.custom_metric(...).
#
# See buildbucket.custom_metric(...) function for all details.
#
# Fields:
#   name: string, custom metric name.
#   predicates: a list of strings with CEL predicate expressions.
#   extra_fields: a string dict with CEL expression for generating metric field values.
_custom_metric_ctor = __native__.genstruct("buildbucket.custom_metric")

def _custom_metric(
        name,
        predicates,
        extra_fields = None):
    """ Defines a custom metric for builds or builders.

    **NOTE**: Before adding a custom metric to your project, you **must** first register it
    in [buildbucket's service config](https://chrome-internal.googlesource.com/infradata/config/+/refs/heads/main/configs/cr-buildbucket/settings.cfg)
    specifying the metric's name, the standard build metric it bases on and
    metric extra_fields, if you plan to add any.

    Then when adding the metric to your project, you **must** specify predicates.

    * If the metric is to report events of all builds under the builder, use the
      standard build metrics instead.

    * If the metric is to report events of all builds under the builder, but with
      extra metric fields, copied from a build metadata, such as tags,
      then add a predicate for the metadata. e.g., `'build.tags.exists(t, t.key=="os")'`.

    Elements in the predicates are concatenated with AND during evaluation, meaning
    the builds must satisfy all predicates to reported to this custom metrics.
    But you could use "||" inside an element to accept a build that satisfys a
    portion of that predicate.

    Each element must be a boolean expression formatted in
    https://github.com/google/cel-spec. Current supported use cases are:
    * a build field have non-empty value, e.g.
      * `'has(build.tags)'`
      * `'has(build.input.properties.out_key)'`
    * a build string field matchs a value, e.g.
      * `'string(build.output.properties.out_key) == "out_val"'`
    * build ends with a specific status: `'build.status.to_string()=="INFRA_FAILURE"'`
    * a specific element exists in a repeated field, e.g.
      * `'build.input.experiments.exists(e, e=="luci.buildbucket.exp")'`
      * `'build.steps.exists(s, s.name=="compile")'`
      * `'build.tags.exists(t, t.key=="os")'`
    * a build tag has a specific value: `'build.tags.get_value("os")=="Linux"'`

    If you specified extra_fields in Buildbucket's service config for the metric,
    you **must** also specify them here.
    For extra_fields:
    * Each key is a metric field.
    * Each value must be a string expression on how to generate that field,
      formatted in https://github.com/google/cel-spec. currently supports:
      * status string value: `'build.status.to_string()'`
      * build experiments concatenated with "|": `'build.experiments.to_string()'`
      * value of a build string field, e.g. `'string(build.input.properties.out_key)'`
      * a tag value: `'build.tags.get_value("branch")'`
      * random string literal: `'"m121"'`

    Additional metric extra_fields that are not listed in the registration in
    buildbucket's service config is allowed, but they will be ignored in
    metric reporting until being added to buildbucket's service config.

    **NOTE**: if you have a predicate or an extra_fields that cannot be supported by
    the CEL expressions we provided, please [file a bug](go/buildbucket-bugs).

    **NOTE**: You could test the predicates and extra_fields using the
    [CustomMetricPreview](https://cr-buildbucket.appspot.com/rpcexplorer/services/buildbucket.v2.Builds/CustomMetricPreview)
    RPC with one example build.

    Args:
      name: custom metric name. It must be a pre-registered custom metric in
        buildbucket's service config. Required.
      predicates: a list of strings with CEL predicate expressions. Required.
      extra_fields: a string dict with CEL expression for generating metric field values.

    Returns:
     buildbucket.custom_metric struct with fields `name`, `predicates` and `extra_fields`.
    """
    return _custom_metric_ctor(
        name = validate.string("name", name, required = True),
        predicates = validate.str_list("predicates", predicates, required = True),
        extra_fields = validate.str_dict("extra_fields", extra_fields, required = False),
    )

def _validate_custom_metrics(attr, custom_metrics):
    """Validates a list of custom metrics.

    Ensure each entry is buildbucket.custom_metric struct.

    Args:
      attr: field name with settings, for error messages. Required.
      custom_metrics: A list of buildbucket_pb.CustomMetricDefinition() protos.

    Returns:
       Validates list of custom metrics (may be an empty list, never None).
    """
    custom_metrics = validate.list(attr, custom_metrics)
    for cm in custom_metrics:
        validate.struct(attr, cm, _custom_metric_ctor)
    return custom_metrics

buildbucket = struct(
    custom_metric = _custom_metric,
    validate_custom_metrics = _validate_custom_metrics,
)
