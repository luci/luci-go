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

import { SxProps, Theme } from '@mui/material';
import ANSIConverter from 'ansi-to-html';
import { useMemo } from 'react';

import { SanitizedHtml } from '../sanitized_html';

const ansiConverter = new ANSIConverter({
  bg: '#FFF',
  fg: '#000',
  newline: true,
  escapeXML: true,
});

export interface ANSITextProps {
  readonly content: string;
  readonly sx?: SxProps<Theme>;
  readonly className?: string;
}

export function ANSIText({ content, ...props }: ANSITextProps) {
  const html = useMemo(() => ansiConverter.toHtml(content), [content]);
  return <SanitizedHtml {...props} html={html} />;
}
