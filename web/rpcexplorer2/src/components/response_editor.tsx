// Copyright 2023 The LUCI Authors.
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

import * as ace from '../ace';

export interface Props {
  value: string,
}

export const ResponseEditor = (props: Props) => {
  return (
    <Box
      component='div'
      sx={{
        border: '1px solid #e0e0e0',
        borderRadius: '2px',
        mb: 2,
      }}
    >
      <ace.AceEditor
        mode={ace.mode}
        theme={ace.theme}
        name='response-editor'
        width='100%'
        height='400px'
        value={props.value}
        setOptions={{
          readOnly: true,
          tabSize: 2,
          dragEnabled: false,
          showPrintMargin: false,
          useWorker: false,
        }}
      />
    </Box>
  );
};
