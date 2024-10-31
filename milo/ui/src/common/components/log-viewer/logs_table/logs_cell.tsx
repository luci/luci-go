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

import { styled } from '@mui/material';
import { grey, red } from '@mui/material/colors';

export const LogsEntryTableCell = styled('td')(({ theme }) => ({
  verticalAlign: 'top',
  padding: 0,
  textAlign: 'left',
  boxSizing: 'content-box',
  fontSize: '12px',
  fontFamily: 'monospace',
  '.line': {
    width: '30px',
    height: '16px',
    display: 'inline-block',
  },
  '.severity': {
    width: '14px',
    height: '16px',
    textAlign: 'center',
    display: 'inline-block',
  },
  '.severity.verbose': {
    backgroundColor: theme.palette.mode === 'light' ? grey[400] : grey[800],
  },
  '.severity.debug': {
    backgroundColor:
      theme.palette.mode === 'light'
        ? theme.palette.success.light
        : theme.palette.success.dark,
  },
  '.severity.info': {
    backgroundColor:
      theme.palette.mode === 'light'
        ? theme.palette.info.light
        : theme.palette.info.dark,
  },
  '.severity.notice': {
    backgroundColor:
      theme.palette.mode === 'light'
        ? theme.palette.warning.light
        : theme.palette.warning.dark,
  },
  '.severity.warning': {
    backgroundColor: theme.palette.warning.main,
    color: 'white',
  },
  '.severity.error': {
    backgroundColor:
      theme.palette.mode === 'light'
        ? theme.palette.error.light
        : theme.palette.error.dark,
    color: theme.palette.mode === 'light' ? 'white' : 'black',
  },
  '.severity.fatal': {
    backgroundColor: theme.palette.error.main,
    color: 'white',
  },
  '.summary': {
    whiteSpace: 'pre-wrap',
    display: 'inline-block',
  },
  '.summary.highlighted': {
    color: theme.palette.mode === 'light' ? red[700] : red[200],
    fontWeight: 'bold',
  },
  '.summary.greyed': {
    color: theme.palette.mode === 'light' ? grey[700] : grey[200],
  },
  '.summary.limit': {
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    display: '-webkit-box',
    lineClamp: 5,
    WebkitLineClamp: 5,
    WebkitBoxOrient: 'vertical',
  },
}));
