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

import { CodeMirrorEditor } from '@/generic_libs/components/code_mirror_editor';

const CODE_SAMPLE = `
import { CodeMirrorEditor } from '@/generic_libs/components/code_mirror_editor';

export function GalleryPage() {
  return (
    <CodeMirrorEditor
      value={JSON.stringify(
        {
          key: 'value',
          nested: {
            key2: 'value2',
          },
        },
        null,
        2,
      )}
    />
  );
}
`.trim();

export function Component() {
  return (
    <div css={{ padding: '10px' }}>
      <h1>Component Gallery</h1>
      <h2>CodeMirrorEditor</h2>
      <p>
        A wrapper around CodeMirror that provides a simple way to display and
        edit code.
      </p>
      <CodeMirrorEditor value={CODE_SAMPLE} />
    </div>
  );
}
