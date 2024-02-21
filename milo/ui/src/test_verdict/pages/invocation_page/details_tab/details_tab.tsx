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
import { DateTime } from 'luxon';
import { useParams } from 'react-router-dom';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { Timestamp } from '@/common/components/timestamp';
import { useTabId } from '@/generic_libs/components/routed_tabs';
import { useResultDbClient } from '@/test_verdict/hooks/prpc_clients';

import { IncludedInvocationsTimeline } from './included_invocations_timeline';

const Container = styled(Box)`
  margin: 10px;
`;

interface TimestampRowProps {
  readonly label: string;
  readonly dateISO?: string;
}

function TimestampRow({ label, dateISO }: TimestampRowProps) {
  return (
    <tr>
      <td>{label}:</td>
      <td>
        {dateISO ? <Timestamp datetime={DateTime.fromISO(dateISO)} /> : 'N/A'}
      </td>
    </tr>
  );
}

export function DetailsTab() {
  const { invId } = useParams();
  if (!invId) {
    throw new Error('invariant violated: invId should be set');
  }

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
