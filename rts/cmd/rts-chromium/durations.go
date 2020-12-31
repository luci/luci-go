// Copyright 2020 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
)

func cmdFetchDurations(authOpt *auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `fetch-durations`,
		ShortDesc: "fetch test duration data",
		LongDesc: text.Doc(`
			Fetch test duration data, suitable for selection strategy evaluation.
			For format details, see comments of TestDurationRecord protobuf message.
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &fetchDurationsRun{}
			r.authOpt = authOpt
			r.RegisterBaseFlags(&r.Flags)
			r.Flags.Float64Var(&r.frac, "frac", 0.001, "Fraction of the data to fetch")
			r.Flags.DurationVar(&r.minDuration, "min-duration", time.Second, "Minimum duration to fetch")
			return r
		},
	}
}

type fetchDurationsRun struct {
	baseCommandRun
	baseHistoryRun
	frac        float64
	minDuration time.Duration
}

func (r *fetchDurationsRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	if len(args) != 0 {
		return r.done(errors.New("unexpected positional arguments"))
	}

	if err := r.baseHistoryRun.Init(ctx); err != nil {
		return r.done(err)
	}

	return r.done(r.runAndFetchResults(
		ctx,
		testDurationsSQL,
		bigquery.QueryParameter{
			Name:  "frac",
			Value: r.frac,
		},
		bigquery.QueryParameter{
			Name:  "minDuration",
			Value: r.minDuration.Seconds(),
		},
	))
}

const testDurationsSQL = `
WITH
	affected_files_raw AS (
		SELECT
			ps.change,
			ps.patchset,
			-- Extract affected files from the property
			-- and replace "src/" prefix with "//".
			ARRAY(
					SELECT REGEXP_REPLACE(JSON_EXTRACT_SCALAR(file_name_json, "$"), "^src/", "//")
					FROM UNNEST(JSON_EXTRACT_ARRAY(b.output.properties, "$.affected_files.first_100")) file_name_json
			) AS files,
		FROM cr-buildbucket.chromium.builds b, b.input.gerrit_changes ps
		WHERE create_time BETWEEN @startTime and TIMESTAMP_ADD(@endTime, INTERVAL 1 DAY)
			-- Skip patchsets that modified >100 files, because we don't have the full
			-- list. This also skips builds that don't have affected flies, e.g. CI builds,
			-- because the value is NULL.
			AND CAST(JSON_EXTRACT(b.output.properties, "$.affected_files.total_count") as FLOAT64) <= 100

			-- Ignore any builds that modified non-src.git files.
			AND NOT EXISTS (
				SELECT 0
				FROM UNNEST(JSON_EXTRACT_ARRAY(b.output.properties, "$.affected_files.first_100")) f
				-- The leading quote is there because it is a JSON string.
				WHERE f NOT LIKE '"src/%'
			)
	),

	affected_files AS (
		-- Choose the longest file list.
		-- File lists for the same patchset can be different if the parent CL landed
		-- between bot_updates of different tryjobs.
		SELECT change, patchset, ARRAY_AGG(af ORDER BY ARRAY_LENGTH(files) DESC LIMIT 1)[OFFSET(0)].files
		FROM affected_files_raw af
		GROUP BY change, patchset
	),

	tryjobs AS (
		SELECT
			b.id,
			ps,
		FROM commit-queue.chromium.attempts a, a.gerrit_changes ps, a.builds b
		WHERE partition_time BETWEEN @startTime AND TIMESTAMP_ADD(@endTime, INTERVAL 1 DAY)
	),
	test_results AS (
		SELECT
			CAST(REGEXP_EXTRACT(exported.id, r'build-(\d+)') as INT64) as build_id,
			IFNULL(test_location.file_name, '') as file_name,
			test_id,
			variant,
			duration,
		FROM luci-resultdb.chromium.try_test_results tr
		WHERE partition_time BETWEEN @startTime and @endTime
			AND RAND() <= @frac
			AND (@test_id_regexp = '' OR REGEXP_CONTAINS(test_id, @test_id_regexp))
			AND (@builder_regexp = '' OR EXISTS (SELECT 0 FROM tr.variant WHERE key='builder' AND REGEXP_CONTAINS(value, @builder_regexp)))
			AND duration > @minDuration

			-- Exclude broken test locations.
			-- TODO(nodir): remove this after crbug.com/1130425 is fixed.
			AND REGEXP_CONTAINS(IFNULL(test_location.file_name, ''), r'(?i)^(|.*\.(cc|html|m|c|cpp))$')
			-- Exclude broken prefixes.
			-- TODO(nodir): remove after crbug.com/1017288 is fixed.
			AND (test_id NOT LIKE 'ninja://:blink_web_tests/%' OR test_location.file_name LIKE '//third_party/%')
	)

-- Join all tables and produce rows in the TestDurationRecord protojson format.
SELECT
	[STRUCT(
		STRUCT(
			"chromium-review.googlesource.com" AS host,
			"chromium/src" AS project,
			ps.change AS number
		) AS change,
		ps.patchset,
		ANY_VALUE(ARRAY(
			SELECT AS STRUCT
				"https://chromium.googlesource.com/chromium/src" AS repo,
				f as path
			FROM UNNEST(af.files) f
		)) AS changedFiles
	)] AS patchsets,
	ARRAY_AGG(STRUCT(
		STRUCT(
			test_id AS id,
			ARRAY(SELECT FORMAT("%s:%s", key, value) kv FROM UNNEST(variant) ORDER BY kv) as variant,
			file_name as fileName
		) as testVariant,
		FORMAT("%fs", duration) as duration
	)) AS testDurations,
FROM tryjobs t
JOIN test_results tr ON t.id = tr.build_id
JOIN affected_files af ON ps.change = af.change AND ps.patchset = af.patchset
GROUP BY ps.change, ps.patchset
`
