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

import { Box, CircularProgress, styled } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { useParams } from 'react-router-dom';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { PropertyViewer } from '@/common/components/property_viewer';
import {
  PropertyViewerConfig,
  PropertyViewerConfigInstance,
} from '@/common/store/user_config/build_config';
import { useTabId } from '@/generic_libs/components/routed_tabs';
import { invocation_StateToJSON } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { useResultDbClient } from '@/test_verdict/hooks/prpc_clients';

import { IncludedInvocationsTimeline } from './included_invocations_timeline';
import { SourceSpecSection } from './source_spec_section';
import { StringRow } from './string_row';
import { TimestampRow } from './timestamp_row';

const Container = styled(Box)`
  margin: 10px;
`;

export function DetailsTab() {
  const { invId } = useParams();
  if (!invId) {
    throw new Error('invariant violated: invId should be set');
  }
  const [propertyViewerConfig, _] = useState<PropertyViewerConfigInstance>(
    PropertyViewerConfig.create({}),
  );

  const client = useResultDbClient();
  const {
    data: invocation,
    error,
    isError,
    isLoading,
  } = useQuery({
    ...client.GetInvocation.query({ name: 'invocations/' + invId }),
  });
  if (isError) {
    throw error;
  }
  if (isLoading) {
    return (
      <Container>
        <Box display="flex" justifyContent="center" alignItems="center">
          <CircularProgress />
        </Box>
      </Container>
    );
  }

  return (
    <Container>
      <h3>General</h3>
      <table>
        <tbody>
          <StringRow
            label="State"
            value={invocation_StateToJSON(invocation.state)}
          />
          <StringRow label="Realm" value={invocation.realm} />
          <StringRow label="Created By" value={invocation.createdBy} />
          <StringRow label="Baseline" value={invocation.baselineId || 'N/A'} />
        </tbody>
      </table>
      <h3>Timing</h3>
      <table>
        <tbody>
          <TimestampRow label="Create Time" dateISO={invocation.createTime} />
          <TimestampRow
            label="Finalize Time"
            dateISO={invocation.finalizeTime}
          />
          <TimestampRow label="Deadline" dateISO={invocation.deadline} />
        </tbody>
      </table>
      {invocation.sourceSpec && (
        <>
          <h3>Source Specification</h3>
          <SourceSpecSection sourceSpec={invocation.sourceSpec} />
        </>
      )}
      {invocation.properties !== undefined && (
        <>
          <h3>Properties</h3>
          <PropertyViewer
            properties={invocation.properties}
            config={propertyViewerConfig}
          />
        </>
      )}
      {invocation.tags.length ? (
        <>
          <h3>Tags</h3>
          <table>
            <tbody>
              {invocation.tags.map((tag, i) => (
                <tr key={i}>
                  <td>{tag.key}=</td>
                  <td>{tag.value}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      ) : (
        <></>
      )}
      {invocation.includedInvocations.length ? (
        <>
          <h3>
            Included Invocations ({invocation.includedInvocations.length}):
          </h3>
          <IncludedInvocationsTimeline invocation={invocation} />
        </>
      ) : (
        <></>
      )}
    </Container>
  );
}

export function Component() {
  useTabId('invocation-details');

  return (
    // See the documentation for `<LoginPage />` for why we handle error this way.
    <RecoverableErrorBoundary key="invocation-details">
      <DetailsTab />
    </RecoverableErrorBoundary>
  );
}
