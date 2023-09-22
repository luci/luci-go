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
import Divider from '@mui/material/Divider';
import Link from '@mui/material/Link';
import Typography from '@mui/material/Typography';

import ReactMarkdown from 'react-markdown';

// Mapping of elements emitted by react-markdown into MUI typography ones for
// consistent style. There are probably some gaps here.
/* eslint-disable @typescript-eslint/no-explicit-any */
const components = {
  a: Link,
  h1: (props: any) => <>
    <Typography component='h1' variant='subtitle1' {...props} />
    <Divider sx={{ mb: 2 }} />
  </>,
  h2: (props: any) => <>
    <Typography component='h2' variant='subtitle1' {...props} />
    <Divider light={true} sx={{ mb: 2 }} />
  </>,
  h3: (props: any) => <>
    <Typography component='h3' variant='subtitle2' {...props} />
    <Divider light={true} sx={{ mb: 2 }} />
  </>,
  h4: (props: any) => <>
    <Typography component='h4' variant='caption' {...props} />
    <Divider light={true} sx={{ mb: 2 }} />
  </>,
  p: (props: any) => <Typography variant='body2' paragraph={true} {...props} />,
  li: (props: any) => <Typography component='li' variant='body2' {...props} />,
};
/* eslint-enable @typescript-eslint/no-explicit-any */


export interface Props {
  markdown: string;
}

export const Doc = ({ markdown }: Props) => {
  if (!markdown) {
    return <></>;
  }
  return (
    <Box ml={2} mr={2} mt={2}>
      <ReactMarkdown skipHtml={true} components={components}>
        {markdown}
      </ReactMarkdown>
    </Box>
  );
};


export default Doc;
