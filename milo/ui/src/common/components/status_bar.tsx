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

import { Box, LinearProgress, styled } from '@mui/material';

export interface StatusBarComponent {
  readonly color: string;
  readonly weight: number;
}

export interface StatusBarProps {
  readonly components: readonly StatusBarComponent[];
  readonly loading?: boolean;
}

const Container = styled(Box)(() => ({
  display: 'flex',
  height: '5px',
  width: '100%',
}));

const Segment = styled(Box, {
  shouldForwardProp: (prop) => prop !== 'weight' && prop !== 'color',
})<{ weight: number; color: string }>(({ weight, color }) => ({
  flexGrow: weight,
  backgroundColor: color,
  height: '100%',
}));

export function StatusBar({ components, loading }: StatusBarProps) {
  if (loading) {
    return (
      <LinearProgress
        sx={{
          height: '5px',
          '& .MuiLinearProgress-bar': {
            backgroundColor: 'var(--active-color)',
          },
          backgroundColor: 'var(--divider-color)',
        }}
      />
    );
  }

  return (
    <Container data-testid="status-bar-container">
      {components.map((c, i) => (
        <Segment
          key={i}
          weight={c.weight}
          color={c.color}
          data-testid="status-bar-segment"
        />
      ))}
    </Container>
  );
}
