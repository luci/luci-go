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

import { Box, Link, styled } from '@mui/material';
import { DateTime } from 'luxon';
import { useMemo } from 'react';

import { Timestamp } from '@/common/components/timestamp';
import { parseArtifactName } from '@/common/services/resultdb';
import { getRawArtifactURLPath, getInvURLPath } from '@/common/tools/url_utils';
import { OutputArtifactMatchingContent } from '@/common/types/verdict';
import { ArtifactMatchingContent_Match } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import {
  TestStatus,
  testStatusToJSON,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { ResultStatusIcon } from '@/test_verdict/components/result_status_icon';

const Container = styled(Box)`
  display: flex;
  gap: 20px;
  border-bottom: 1px solid #dadce0;
  padding: 2px 20px;
  font-size: 15px;
  line-height: 1.5;
  letter-spacing: normal;
`;

interface Token {
  readonly type: 'matching' | 'non-matching';
  readonly content: string;
}

function splitSnippet(
  snippet: string,
  matches: readonly ArtifactMatchingContent_Match[],
) {
  // matches uses the byte indexing, javascript uses UTF-16 encoding for strings.
  // To apply the index from match to snippet correct, we need to convert the snippet into UTF-8 first.
  const encoder = new TextEncoder();
  const decoder = new TextDecoder();
  const utf8Bytes = encoder.encode(snippet);

  const tokens: Token[] = [];
  let currentIndex = 0;
  for (const { startIndex, endIndex } of matches) {
    // Add part before a match.
    if (currentIndex < startIndex) {
      tokens.push({
        type: 'non-matching',
        content: decoder.decode(utf8Bytes.slice(currentIndex, startIndex)),
      });
    }
    // Add a match part.
    tokens.push({
      type: 'matching',
      content: decoder.decode(utf8Bytes.slice(startIndex, endIndex)),
    });
    currentIndex = endIndex;
  }
  // Add the remaining part after the last match.
  if (currentIndex < utf8Bytes.length) {
    tokens.push({
      type: 'non-matching',
      content: decoder.decode(utf8Bytes.slice(currentIndex)),
    });
  }
  return tokens;
}

function getLogViewerLink(
  artifactName: string,
  match?: string,
  variantHash?: string | null,
) {
  const { invocationId, testId, resultId, artifactId } =
    parseArtifactName(artifactName);
  if (!(testId && resultId && variantHash && match)) {
    return null;
  }
  const search = new URLSearchParams();
  search.set('resultId', resultId);
  search.set('topPanelExpanded', '0');
  search.set('logSearchQuery', match);
  search.set('selectedArtifact', artifactId);
  search.set('selectedArtifactSource', 'result');
  return `/ui/test-investigate/invocations/${invocationId}/tests/${encodeURIComponent(testId)}/variants/${variantHash}/?${search}`;
}

export interface LogSnippetRowProps {
  readonly artifact: OutputArtifactMatchingContent;
  readonly variantHash?: string | null;
}

export function LogSnippetRow({ artifact, variantHash }: LogSnippetRowProps) {
  const { name, partitionTime, testStatus, snippet, matches } = artifact;
  const tokens: Token[] = useMemo(
    () => splitSnippet(snippet, matches),
    [snippet, matches],
  );
  const firstMatch = tokens.find((token) => token.type === 'matching');
  const logViewerLink = getLogViewerLink(
    artifact.name,
    firstMatch?.content,
    variantHash,
  );
  const invocationID = getInvocationID(name);
  return (
    <Container>
      <Box sx={{ flex: 'none', width: '100px' }}>
        <Timestamp
          datetime={DateTime.fromISO(partitionTime!)}
          format="MMM dd, HH:mm"
        />
        {testStatus !== TestStatus.STATUS_UNSPECIFIED && (
          <Box
            sx={{
              alignItems: 'center',
              display: 'flex',
            }}
          >
            <ResultStatusIcon status={testStatus} sx={{ fontSize: '22px' }} />
            {testStatusToJSON(testStatus)}
          </Box>
        )}
        {logViewerLink === null && invocationID !== null && (
          // Link to the invocation page when the log view link is not available.
          <Box
            sx={{
              alignItems: 'center',
              display: 'flex',
            }}
          >
            <Link
              href={getInvURLPath(invocationID)}
              target="_blank"
              rel="noopenner"
            >
              invocation
            </Link>
          </Box>
        )}
      </Box>
      <Link
        // log viewer is only available for test result level logs.
        // Fallback to the raw artifact when it is not available.
        href={logViewerLink || getRawArtifactURLPath(name)}
        underline="none"
        color="inherit"
        target="_blank"
        rel="noopenner"
      >
        <Box
          sx={{
            whiteSpace: 'pre-line',
            wordBreak: 'break-all',
          }}
        >
          {tokens.map((token, i) => {
            return token.type === 'non-matching' ? (
              <span key={i}>{token.content}</span>
            ) : (
              <mark
                key={i}
                css={{ backgroundColor: '#ceead6', fontWeight: '700' }}
              >
                {token.content}
              </mark>
            );
          })}
        </Box>
      </Link>
    </Container>
  );
}

function getInvocationID(artifactName: string) {
  const match = artifactName.match(/invocations\/([^/]+)/);
  return match && match.length > 0 ? match[1] : null;
}
