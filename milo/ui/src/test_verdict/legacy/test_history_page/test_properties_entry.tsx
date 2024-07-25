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

import { ModeSpec } from 'codemirror';
import { useState } from 'react';

import { extractProject } from '@/common/tools/utils';
import { CodeMirrorEditor } from '@/generic_libs/components/code_mirror_editor';
import {
  ExpandableEntry,
  ExpandableEntryBody,
  ExpandableEntryHeader,
} from '@/generic_libs/components/expandable_entry';
import { QueryTestMetadataRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';

import { useTestMetadata } from './utils';

export interface TestPropertiesEntryProps {
  readonly projectOrRealm: string; // A project name or a realm name.
  readonly testId: string;
}
export function TestPropertiesEntry({
  projectOrRealm,
  testId,
}: TestPropertiesEntryProps) {
  const [testPropertiesExpanded, setTestPropertiesExpanded] = useState(false);
  const project = extractProject(projectOrRealm);
  const { data, isSuccess, isLoading } = useTestMetadata(
    QueryTestMetadataRequest.fromPartial({
      project,
      predicate: { testIds: [testId] },
    }),
  );
  return (
    <>
      {!isLoading && isSuccess && data?.testMetadata?.properties && (
        <ExpandableEntry expanded={testPropertiesExpanded}>
          <div css={{ color: 'var(--light-text-color)' }}>
            <ExpandableEntryHeader onToggle={setTestPropertiesExpanded}>
              Test Properties
            </ExpandableEntryHeader>
          </div>
          <ExpandableEntryBody ruler="none">
            <CodeMirrorEditor
              value={JSON.stringify(data.testMetadata.properties, undefined, 2)}
              initOptions={{
                mode: { name: 'javascript', json: true } as ModeSpec<{
                  json: boolean;
                }>,
                readOnly: true,
                matchBrackets: true,
                lineWrapping: true,
                foldGutter: true,
                lineNumbers: true,
                // Ensures all nodes are rendered therefore searchable.
                viewportMargin: Infinity,
                gutters: ['CodeMirror-linenumbers', 'CodeMirror-foldgutter'],
              }}
            />
          </ExpandableEntryBody>
        </ExpandableEntry>
      )}
    </>
  );
}
