// Copyright 2022 The LUCI Authors.
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

import Box from '@mui/material/Box';
import Grid from '@mui/material/Grid2';

interface Props {
  text?: string;
  children?: React.ReactNode;
  xs?: number;
  lg?: number;
  testid?: string;
}

const GridLabel = ({ text, children, xs = 2, lg = xs, testid }: Props) => {
  return (
    <Grid
      data-testid={testid}
      size={{
        xs: xs,
        lg: lg,
      }}
    >
      <Box
        sx={{
          display: 'inline-block',
          wordBreak: 'break-all',
          overflowWrap: 'break-word',
        }}
        paddingTop={1}
      >
        {text}
      </Box>
      {children}
    </Grid>
  );
};

export default GridLabel;
