// Copyright 2025 The LUCI Authors.
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
import { ComponentType, useMemo } from 'react';

import {
  BuildCheckOptionsDetails,
  BuildCheckResultDetails,
} from './build_details';
import { GenericJsonDetails } from './generic_json_details';
import {
  GobSourceCheckOptionsDetails,
  GobSourceCheckResultsDetails,
} from './gob_details';
import { PiperSourceCheckOptionsDetails } from './piper_details';
import {
  TestCheckDescriptionOptionDetails,
  TestCheckSummaryResultDetails,
} from './test_details';

// Define a generic renderer type that accepts unknown data.
// Data must be unknown in order to support multiple different protos.
type GenericRenderer = ComponentType<{ data: unknown }>;

// Map known type URLs to their specialized renderer components.
// Data type is unknown because it could be any of the known protos.
const KNOWN_TYPE_RENDERERS: Record<string, GenericRenderer> = {
  'type.googleapis.com/turboci.data.build.v1.BuildCheckOptions':
    BuildCheckOptionsDetails as unknown as GenericRenderer,
  'type.googleapis.com/turboci.data.build.v1.BuildCheckResult':
    BuildCheckResultDetails as unknown as GenericRenderer,
  'type.googleapis.com/turboci.data.gerrit.v1.GobSourceCheckOptions':
    GobSourceCheckOptionsDetails as unknown as GenericRenderer,
  'type.googleapis.com/turboci.data.gerrit.v1.GobSourceCheckResults':
    GobSourceCheckResultsDetails as unknown as GenericRenderer,
  'type.googleapis.com/turboci.data.piper.v1.PiperSourceCheckOptions':
    PiperSourceCheckOptionsDetails as unknown as GenericRenderer,
  'type.googleapis.com/turboci.data.test.v1.TestCheckDescriptionOption':
    TestCheckDescriptionOptionDetails as unknown as GenericRenderer,
  'type.googleapis.com/turboci.data.test.v1.TestCheckSummaryResult':
    TestCheckSummaryResultDetails as unknown as GenericRenderer,
};

export interface AnyDetailsProps {
  /** The type URL of the data. */
  typeUrl?: string;
  /** The JSON string data. */
  json?: string;
  /** An optional label to show above the field. */
  label?: string;
}

/**
 * Renders details for a given JSON data blob, using a specialized component if available for the type,
 * otherwise falling back to a generic JSON view.
 */
export function AnyDetails({ typeUrl, json, label }: AnyDetailsProps) {
  const data = useMemo(() => {
    if (!json) return <ParseError />;
    try {
      return JSON.parse(json);
    } catch {
      return <ParseError />;
    }
  }, [json]);

  if (!data) return <ParseError />;

  const Renderer = typeUrl ? KNOWN_TYPE_RENDERERS[typeUrl] : undefined;
  if (Renderer && data) {
    return (
      <Box sx={{ mt: 1 }}>
        {typeUrl && (
          <Typography variant="caption" color="text.secondary">
            {label ? `${label}: ` : ''}
            {typeUrl}
          </Typography>
        )}
        <Box sx={{ p: 1, border: '1px solid #eee', borderRadius: 1, mt: 0.5 }}>
          <Renderer data={data} />
        </Box>
      </Box>
    );
  }

  return <GenericJsonDetails label={label} typeUrl={typeUrl} json={json} />;
}

function ParseError() {
  return <Typography variant="caption">Failed to parse data.</Typography>;
}
