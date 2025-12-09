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

import { Box } from '@mui/material';

import { CodeMirrorEditor } from '@/generic_libs/components/code_mirror_editor';
import { useDeclareTabId } from '@/generic_libs/components/routed_tabs';
import { useInvocation } from '@/test_investigation/context/context';

export function PropertiesTab() {
  useDeclareTabId('properties');
  const invocation = useInvocation();

  const properties = invocation.properties || {};
  const formattedProperties = JSON.stringify(properties, null, 2);

  return (
    <Box sx={{ p: 2 }}>
      <CodeMirrorEditor
        value={formattedProperties}
        initOptions={{
          mode: 'javascript',
          readOnly: true,
          lineNumbers: true,
          foldGutter: true,
          gutters: ['CodeMirror-linenumbers', 'CodeMirror-foldgutter'],
        }}
      />
    </Box>
  );
}
