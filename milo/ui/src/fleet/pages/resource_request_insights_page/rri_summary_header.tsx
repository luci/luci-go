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

import styled from '@emotion/styled';
import { Typography } from '@mui/material';

import { SingleMetric } from '@/fleet/components/summary_header/single_metric';
import { colors } from '@/fleet/theme/colors';

const Container = styled.div`
  padding: 16px 21px;
  gap: 28;
  border: 1px solid ${colors.grey[300]};
  border-radius: 4;
`;

export function RriSummaryHeader() {
  return (
    <Container>
      <Typography variant="h4">My Requests</Typography>
      <div css={{ marginTop: 24 }}>
        <Typography variant="subhead1">Task status</Typography>
        <div
          css={{
            display: 'flex',
            justifyContent: 'space-around',
            marginTop: 5,
          }}
        >
          <SingleMetric name="In Progress" value={7} total={9} />
          <SingleMetric name="Completed" value={2} total={9} />
          <SingleMetric name="Material Sourcing" value={2} total={9} />
          <SingleMetric name="Build" value={2} total={9} />
          <SingleMetric name="QA" value={1} total={9} />
          <SingleMetric name="Config" value={1} total={9} />
        </div>
      </div>
    </Container>
  );
}
