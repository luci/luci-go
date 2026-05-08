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

import { Box, Typography } from '@mui/material';
import { useState } from 'react';

import {
  ExpandableEntry,
  ExpandableEntryBody,
  ExpandableEntryHeader,
} from '@/generic_libs/components/expandable_entry';
import {
  FailureReason,
  FailureReason_Error,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/failure_reason.pb';

export interface FailureReasonEntryProps {
  readonly failureReason: FailureReason;
  readonly inline?: boolean;
}

interface ErrorDisplayProps {
  readonly error: FailureReason_Error;
  readonly isPrimary?: boolean;
}

function ErrorDisplay({ error, isPrimary = false }: ErrorDisplayProps) {
  const [showTrace, setShowTrace] = useState(isPrimary);
  const hasTrace = Boolean(error.trace);

  return (
    <Box sx={{ mb: 1 }}>
      <pre
        css={{
          backgroundColor: 'var(--block-background-color)',
          padding: '5px',
          margin: 0,
          whiteSpace: 'pre-wrap',
          overflowWrap: 'break-word',
        }}
      >
        {error.message}
      </pre>
      {hasTrace && (
        <Box sx={{ mt: 0.5 }}>
          <ExpandableEntry expanded={showTrace}>
            <ExpandableEntryHeader
              onToggle={(expanded) => setShowTrace(expanded)}
            >
              Stack Trace
            </ExpandableEntryHeader>
            <ExpandableEntryBody>
              <pre
                css={{
                  backgroundColor: 'var(--block-background-color)',
                  padding: '5px',
                  margin: '4px 0 0 0',
                  color: 'var(--greyed-out-text-color, #666)',
                  fontSize: '11px',
                  whiteSpace: 'pre-wrap',
                  overflowWrap: 'break-word',
                  borderLeft: '2px solid var(--divider-color, #ccc)',
                  paddingLeft: '8px',
                }}
              >
                {error.trace}
              </pre>
            </ExpandableEntryBody>
          </ExpandableEntry>
        </Box>
      )}
    </Box>
  );
}

export function FailureReasonEntry({
  failureReason,
  inline = false,
}: FailureReasonEntryProps) {
  const [expanded, setExpanded] = useState(true);
  const [additionalExpanded, setAdditionalExpanded] = useState(false);

  const errors = failureReason.errors || [];
  const primaryError = errors[0];
  const additionalErrors = errors.slice(1);
  const truncatedCount = failureReason.truncatedErrorsCount || 0;
  const hasAdditionalErrors = additionalErrors.length > 0 || truncatedCount > 0;
  const totalAdditionalCount = additionalErrors.length + truncatedCount;

  const body = (
    <>
      {/* Primary Error */}
      {primaryError ? (
        <ErrorDisplay error={primaryError} isPrimary={true} />
      ) : (
        // Fallback for older clients
        <pre
          css={{
            backgroundColor: 'var(--block-background-color)',
            padding: '5px',
            margin: 0,
            whiteSpace: 'pre-wrap',
            overflowWrap: 'break-word',
          }}
        >
          {failureReason.primaryErrorMessage}
        </pre>
      )}

      {/* Additional Errors */}
      {hasAdditionalErrors && (
        <Box sx={{ mt: 1.5 }}>
          <ExpandableEntry expanded={additionalExpanded}>
            <ExpandableEntryHeader
              onToggle={(expanded) => setAdditionalExpanded(expanded)}
            >
              Additional Errors ({totalAdditionalCount} more)
            </ExpandableEntryHeader>
            <ExpandableEntryBody>
              {additionalErrors.map((err, idx) => (
                <Box
                  key={idx}
                  sx={{
                    mt: 1,
                    pl: 1,
                    borderLeft: '2px solid var(--divider-color, #ccc)',
                  }}
                >
                  <Typography
                    variant="subtitle2"
                    color="error"
                    sx={{ mb: 0.5 }}
                  >
                    Error {idx + 2}
                  </Typography>
                  <ErrorDisplay error={err} />
                </Box>
              ))}
              {truncatedCount > 0 && (
                <Box
                  sx={{
                    mt: 1,
                    color: 'warning.main',
                    display: 'flex',
                    alignItems: 'center',
                    gap: 0.5,
                    fontSize: '12px',
                  }}
                >
                  ⚠️ {truncatedCount} errors were truncated due to size limits.
                  View full logs for details.
                </Box>
              )}
            </ExpandableEntryBody>
          </ExpandableEntry>
        </Box>
      )}
    </>
  );

  if (inline) {
    return body;
  }

  return (
    <ExpandableEntry expanded={expanded}>
      <ExpandableEntryHeader onToggle={(expanded) => setExpanded(expanded)}>
        Failure Reason:
      </ExpandableEntryHeader>
      <ExpandableEntryBody>{body}</ExpandableEntryBody>
    </ExpandableEntry>
  );
}
