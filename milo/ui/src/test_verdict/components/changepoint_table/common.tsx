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

import { Box, styled } from '@mui/material';

import { LINE_HEIGHT } from './constants';

export const LabelBox = styled(Box)`
  padding: 2px 4px;
  box-sizing: border-box;
  overflow: hidden;
  text-overflow: ellipsis;
  text-wrap: nowrap;
  height: ${LINE_HEIGHT}px;
`;
