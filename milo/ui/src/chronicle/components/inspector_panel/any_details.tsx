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

import { Box, Typography, Alert } from '@mui/material';
import { ComponentType, ReactNode, useMemo } from 'react';

import { BuildCheckOptions } from '@/proto/turboci/data/build/v1/build_check_options.pb';
import { BuildCheckResult } from '@/proto/turboci/data/build/v1/build_check_results.pb';
import { GobSourceCheckOptions } from '@/proto/turboci/data/gerrit/v1/gob_source_check_options.pb';
import { GobSourceCheckResults } from '@/proto/turboci/data/gerrit/v1/gob_source_check_results.pb';
import { PiperSourceCheckOptions } from '@/proto/turboci/data/piper/v1/piper_source_check_options.pb';
import { CommonStageAttemptDetails as CommonStageAttemptDetailsProto } from '@/proto/turboci/data/stage/v1/common_stage_attempt_details.pb';
import { TestCheckDescriptionOption } from '@/proto/turboci/data/test/v1/test_check_description_option.pb';
import { TestCheckSummaryResult } from '@/proto/turboci/data/test/v1/test_check_summary_result.pb';
import { OmitReason } from '@/proto/turboci/graph/orchestrator/v1/omit_reason.pb';
import {
  DataConversionFailure,
  ValueData,
  dataConversionFailureToJSON,
} from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';

import {
  BuildCheckOptionsDetails,
  BuildCheckResultDetails,
} from './build_details';
import { CommonStageAttemptDetails } from './common_stage_attempt_details';
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

interface TypeRenderer {
  component: GenericRenderer;
  fromJson?: (object: unknown) => unknown;
}

// Map known type URLs to their specialized renderer components.
// Data type is unknown because it could be any of the known protos.
const KNOWN_TYPE_RENDERERS: Record<string, TypeRenderer> = {
  'type.googleapis.com/turboci.data.build.v1.BuildCheckOptions': {
    component: BuildCheckOptionsDetails as unknown as GenericRenderer,
    fromJson: BuildCheckOptions.fromJSON,
  },
  'type.googleapis.com/turboci.data.build.v1.BuildCheckResult': {
    component: BuildCheckResultDetails as unknown as GenericRenderer,
    fromJson: BuildCheckResult.fromJSON,
  },
  'type.googleapis.com/turboci.data.gerrit.v1.GobSourceCheckOptions': {
    component: GobSourceCheckOptionsDetails as unknown as GenericRenderer,
    fromJson: GobSourceCheckOptions.fromJSON,
  },
  'type.googleapis.com/turboci.data.gerrit.v1.GobSourceCheckResults': {
    component: GobSourceCheckResultsDetails as unknown as GenericRenderer,
    fromJson: GobSourceCheckResults.fromJSON,
  },
  'type.googleapis.com/turboci.data.piper.v1.PiperSourceCheckOptions': {
    component: PiperSourceCheckOptionsDetails as unknown as GenericRenderer,
    fromJson: PiperSourceCheckOptions.fromJSON,
  },
  'type.googleapis.com/turboci.data.stage.v1.CommonStageAttemptDetails': {
    component: CommonStageAttemptDetails as unknown as GenericRenderer,
    fromJson: CommonStageAttemptDetailsProto.fromJSON,
  },
  'type.googleapis.com/turboci.data.test.v1.TestCheckDescriptionOption': {
    component: TestCheckDescriptionOptionDetails as unknown as GenericRenderer,
    fromJson: TestCheckDescriptionOption.fromJSON,
  },
  'type.googleapis.com/turboci.data.test.v1.TestCheckSummaryResult': {
    component: TestCheckSummaryResultDetails as unknown as GenericRenderer,
    fromJson: TestCheckSummaryResult.fromJSON,
  },
};

export interface AnyDetailsProps {
  /** The type URL of the data. */
  typeUrl?: string;
  /** The JSON string data. */
  json?: string;
  /** An optional label to show above the field. */
  label?: string;
  /** The reason why this value was omitted, if any. */
  omitReason?: OmitReason;
  /** The actual ValueData object containing JSON or binary or conversionFailure. */
  valueData?: ValueData;
}

/**
 * Renders details for a given JSON data blob, using a specialized component if available for the type,
 * otherwise falling back to a generic JSON view.
 *
 * If the data was omitted, renders a clear notice explaining why.
 */
export function AnyDetails({
  typeUrl,
  json,
  label,
  omitReason,
  valueData,
}: AnyDetailsProps) {
  const parsedData = useMemo(() => {
    if (!json) return null;
    try {
      return JSON.parse(json);
    } catch {
      return null;
    }
  }, [json]);

  if (
    omitReason !== undefined &&
    omitReason !== OmitReason.OMIT_REASON_UNKNOWN
  ) {
    return (
      <OmittedValueNotice reason={omitReason} typeUrl={typeUrl} label={label} />
    );
  }

  if (
    valueData?.conversionFailure ===
    DataConversionFailure.DATA_CONVERSION_FAILURE_NO_DESCRIPTOR
  ) {
    return <NoDescriptorNotice typeUrl={typeUrl} label={label} />;
  }

  if (valueData?.conversionFailure) {
    return (
      <ConversionErrorNotice
        typeUrl={typeUrl}
        label={label}
        conversionFailure={valueData.conversionFailure}
      />
    );
  }

  if (parsedData === null) {
    return <ParseError />;
  }

  const typeRenderer = typeUrl ? KNOWN_TYPE_RENDERERS[typeUrl] : undefined;
  if (typeRenderer) {
    const { component: Renderer, fromJson } = typeRenderer;
    const typedData = fromJson ? fromJson(parsedData) : parsedData;
    return (
      <Box sx={{ mt: 1 }}>
        {typeUrl && (
          <Typography variant="caption" color="text.secondary">
            {label ? `${label}: ` : ''}
            {typeUrl}
          </Typography>
        )}
        <Box sx={{ p: 1, border: '1px solid #eee', borderRadius: 1, mt: 0.5 }}>
          <Renderer data={typedData} />
        </Box>
      </Box>
    );
  }

  return <GenericJsonDetails label={label} typeUrl={typeUrl} json={json} />;
}

interface AlertNoticeProps {
  severity: 'error' | 'warning' | 'info' | 'success';
  typeUrl?: string;
  label?: string;
  children: ReactNode;
}

function AlertNotice({ severity, typeUrl, label, children }: AlertNoticeProps) {
  return (
    <Box sx={{ mt: 1 }}>
      {typeUrl && (
        <Typography variant="caption" color="text.secondary">
          {label ? `${label}: ` : ''}
          {typeUrl}
        </Typography>
      )}
      <Alert severity={severity} sx={{ mt: 0.5 }}>
        {children}
      </Alert>
    </Box>
  );
}

function OmittedValueNotice({
  reason,
  typeUrl,
  label,
}: {
  reason: OmitReason;
  typeUrl?: string;
  label?: string;
}) {
  let text = 'Data omitted';
  let severity: 'error' | 'warning' | 'info' = 'info';

  switch (reason) {
    case OmitReason.OMIT_REASON_NO_ACCESS:
      text = 'Access Denied: You do not have permission to view this data.';
      severity = 'error';
      break;
    case OmitReason.OMIT_REASON_MISSING:
      text =
        'Data Missing: This data is no longer available in the storage backend (it may have expired).';
      severity = 'warning';
      break;
    case OmitReason.OMIT_REASON_UNWANTED:
      text = 'Omitted: This data was excluded by the UI query filter.';
      severity = 'info';
      break;
  }

  return (
    <AlertNotice severity={severity} typeUrl={typeUrl} label={label}>
      {text}
    </AlertNotice>
  );
}

function NoDescriptorNotice({
  typeUrl,
  label,
}: {
  typeUrl?: string;
  label?: string;
}) {
  return (
    <AlertNotice severity="warning" typeUrl={typeUrl} label={label}>
      JSON content could not be retrieved, because the Turbo CI orchestrator did
      not have a descriptor for type{' '}
      {typeUrl ? <code>{typeUrl}</code> : 'unknown type'}. You may need to
      register the proto with a stage executor.
    </AlertNotice>
  );
}

function ConversionErrorNotice({
  typeUrl,
  label,
  conversionFailure,
}: {
  typeUrl?: string;
  label?: string;
  conversionFailure: DataConversionFailure;
}) {
  const failureText = dataConversionFailureToJSON(conversionFailure);

  return (
    <AlertNotice severity="error" typeUrl={typeUrl} label={label}>
      The Turbo CI orchestrator failed to convert binary content to JSON for
      type {typeUrl ? <code>{typeUrl}</code> : 'unknown type'} due to{' '}
      <code>{failureText}</code>.
    </AlertNotice>
  );
}

function ParseError() {
  return <Typography variant="caption">Failed to parse data.</Typography>;
}
