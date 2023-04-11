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

import {
  useEffect, useRef, forwardRef, useImperativeHandle, ForwardedRef,
} from 'react';

import Box from '@mui/material/Box';

import AceEditor from 'react-ace';

// Note: these must be imported after AceEditor for some reason, otherwise the
// final bundle ends up broken.
import 'ace-builds/src-noconflict/ext-language_tools';
import 'ace-builds/src-noconflict/ext-searchbox';
import 'ace-builds/src-noconflict/ext-prompt';
import 'ace-builds/src-noconflict/mode-json';
import 'ace-builds/src-noconflict/theme-tomorrow';

export interface Props {
  defaultValue: string;
  readOnly: boolean;
  onInvokeMethod: () => void;
}

export interface RequestEditorRef {
  prepareRequest(): object;
}

export const RequestEditor = forwardRef((
    props: Props, ref: ForwardedRef<RequestEditorRef>) => {
  const editorRef = useRef<AceEditor>(null);

  // Configure the ACE editor on initial load.
  useEffect(() => {
    if (!editorRef.current) {
      return;
    }
    const editor = editorRef.current.editor;

    // Send the request when hitting Shift+Enter.
    editor.commands.addCommand({
      name: 'execute-request',
      bindKey: { win: 'Shift-Enter', mac: 'Shift-Enter' },
      exec: props.onInvokeMethod,
    });
  }, [props.onInvokeMethod]);

  // Expose some methods on the ref assigned to RequestEditor. We do it this
  // way to avoid using AceEditor in a "managed" mode, since it can get slow
  // with large requests.
  useImperativeHandle(ref, () => ({
    prepareRequest: () => {
      if (!editorRef.current) {
        throw new Error('Uninitialized RequestEditor');
      }
      const editor = editorRef.current.editor;
      // Default to an empty request.
      let requestBody = editor.getValue().trim();
      if (!requestBody) {
        requestBody = '{}';
        editor.setValue(requestBody, -1);
      }
      // The request must be a valid JSON. Verify locally by parsing.
      return JSON.parse(requestBody);
    },
  }));

  return (
    <Box
      component='div'
      sx={{ border: '1px solid #e0e0e0', borderRadius: '2px' }}
    >
      <AceEditor
        ref={editorRef}
        mode='json'
        defaultValue={props.defaultValue}
        theme='tomorrow'
        name='request-editor'
        width='100%'
        height='200px'
        setOptions={{
          readOnly: props.readOnly,
          useWorker: false,
          tabSize: 2,
        }}
      />
    </Box>
  );
});

// Eslint wants this.
RequestEditor.displayName = 'RequestEditor';
