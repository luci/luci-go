// Copyright 2024 The LUCI Authors.
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

package artifacts

import (
	"bytes"
	"context"
	"regexp"

	"cloud.google.com/go/bigquery"
	farm "github.com/leemcloughlin/gofarmhash"
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth/realms"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// normalizeRegex is a regex to normalize a log line by removing things that are likely to change between test runs, like timestamps, numbers, ip addresses, tmp file names, etc
var normalizePattern = `(?:(?:(Sat|Sun|Mon|Tue|Wed|Thu|Fri|Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec|UTC|PDT|PST)[,\s]+))|(?:(?:([A-Fa-f0-9]{1,4}::?){1,7}[A-Fa-f0-9]{1,4}))|(?:(?:([A-Fa-f0-9]{2}[:-]){5}[A-Fa-f0-9]{2}))|(?:(?:https?\:\S*))|(?:(?:\/[user|usr|tmp|dev|home|run|devices|lib|root]+\/\S*))|(?:(?:chromeos\d+-row\d+-\S*|chrome-bot@\S*))|(?:(?:SSID=\S*))|(?:(?:hexdump\(len=.*))|(?:(?:\w{32,}))|(?:(?:0x[A-Fa-f0-9]+|[A-Fa-f0-9x]{8,}|[A-Fa-f0-9]{4,}[\-\_\'\]+|\:[A-Fa-f0-9]{4,}))|(?:(?:(-)?[0-9]+))`
var normalizeRegex = regexp.MustCompile(normalizePattern)

// FetchPassingHashes fetches the content of numPassesToCompare examples of the given log
// file from passing test results and normalizes and then hashes each line in the same
// fashion as HashLine.
//
// This is currently implemented using BigQuery, and the hashing and normalization is done
// inside the query for parralellization and to minimize data transfer.
func FetchPassingHashes(ctx context.Context, client *bigquery.Client, realm string, testID string, artifactID string, numPassesToCompare int) (map[int64]struct{}, error) {
	project, _ := realms.Split(realm)
	q := client.Query(`
	with passes as (
		SELECT
		content
		FROM
		` + "`internal.text_artifacts`" + `
		WHERE
		DATE(partition_time) >= DATE_SUB(CURRENT_DATE(), INTERVAL 1 DAY)
		AND project = @project
		AND test_id = @testID
		AND artifact_id = @artifactID
		AND test_status = 'PASS'
		LIMIT @numPassesToCompare)
		SELECT
		distinct farm_fingerprint(regexp_replace(line, r'` + normalizePattern + `', '')) as PassingHash
		FROM
		UNNEST((
			SELECT
			SPLIT(string_agg(content,'\n'), '\n') AS lines
			FROM
			passes)) line
			`)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "project", Value: project},
		{Name: "testID", Value: testID},
		{Name: "artifactID", Value: artifactID},
		{Name: "numPassesToCompare", Value: numPassesToCompare},
	}
	// Execute the query.
	it, err := q.Read(ctx)
	if err != nil {
		return nil, errors.Fmt("BQ read: %w", err)
	}
	// Iterate through the results.
	passingHashes := map[int64]struct{}{}
	type hashRow struct {
		PassingHash int64
	}
	for {
		var row hashRow
		err := it.Next(&row)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, errors.Fmt("fetching BQ row: %w", err)
		}
		passingHashes[row.PassingHash] = struct{}{}
	}
	return passingHashes, nil
}

// HashLine calculates a hash for a line of a log file.
// It first normalizes the line to increase the probablility that it will match in two different test runs (e.g. removing timestamps).
// It uses the farm.Fingerprint64 hash as it needs to match the hash done in BigQuery above.
func HashLine(line string) int64 {
	normalized := normalizeRegex.ReplaceAllString(line, "")
	return int64(farm.FingerPrint64([]byte(normalized)))
}

// ToFailureOnlyLineRanges fetches the content of an artifact and returns only the line ranges that do not
// hash to one fo the values in passingHashes (using HashLine).
//
// This function will optionally include the content of the failure only lines in the ranges.
func ToFailureOnlyLineRanges(artifactID string, contentType string, content []byte, passingHashes map[int64]struct{}, includeContent bool) ([]*pb.QueryArtifactFailureOnlyLinesResponse_LineRange, error) {
	isSupported := IsLogSupportedArtifact(artifactID, contentType)

	if !isSupported {
		return nil, errors.New("unsupported file type")
	}

	linesContent := bytes.Split(content, []byte("\n"))

	ranges := []*pb.QueryArtifactFailureOnlyLinesResponse_LineRange{}
	rangeStart := -1
	for index, lineContent := range linesContent {
		lineContentStr := string(lineContent)
		h := HashLine(lineContentStr)
		_, present := passingHashes[h]
		if present && rangeStart != -1 {
			// end of a range
			var lines []string
			if includeContent {
				for _, line := range linesContent[rangeStart:index] {
					lines = append(lines, string(line))
				}
			}
			ranges = append(ranges, &pb.QueryArtifactFailureOnlyLinesResponse_LineRange{
				Start: int32(rangeStart),
				End:   int32(index),
				Lines: lines,
			})
			rangeStart = -1
		} else if !present && rangeStart == -1 {
			// start of a range.
			rangeStart = index
		}
	}
	if rangeStart != -1 {
		ranges = append(ranges, &pb.QueryArtifactFailureOnlyLinesResponse_LineRange{
			Start: int32(rangeStart),
			End:   int32(len(linesContent)),
		})
	}

	return ranges, nil
}
