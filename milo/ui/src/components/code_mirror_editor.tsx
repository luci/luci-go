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

import 'codemirror/addon/fold/foldgutter.css';
import 'codemirror/lib/codemirror.css';
import 'codemirror/addon/edit/matchbrackets';
import 'codemirror/addon/fold/brace-fold';
import 'codemirror/addon/fold/foldcode';
import 'codemirror/addon/fold/foldgutter';
import 'codemirror/mode/javascript/javascript';
import styled from '@emotion/styled';
import * as CodeMirror from 'codemirror';
import { useEffect, useRef } from 'react';

const Container = styled.div`
  display: block;
  border-radius: 4px;
  border: 1px solid var(--divider-color);
  overflow: hidden;

  & .CodeMirror {
    height: auto;
    max-height: 1000px;
    font-size: 12px;
  }
  & .CodeMirror-scroll {
    max-height: 1000px;
  }
  & .cm-property.cm-string {
    color: #318495;
  }
  & .cm-string:not(.cm-property) {
    color: #036a06;
  }
`;

export interface CodeMirrorEditorProps {
  readonly value: string;
  /**
   * The editor configuration. The value is only used when initializing the
   * editor. Updates are not applied.
   */
  readonly initOptions?: CodeMirror.EditorConfiguration;
  readonly onChange?: (editor: CodeMirror.Editor) => void;
}

export function CodeMirrorEditor({ value, initOptions, onChange }: CodeMirrorEditorProps) {
  const textAreaRef = useRef<HTMLTextAreaElement | null>(null);
  const editorRef = useRef<CodeMirror.EditorFromTextArea | null>(null);

  useEffect(() => {
    // This will never happen, but useful for type checking.
    if (!textAreaRef.current) {
      return;
    }

    if (editorRef.current) {
      return;
    }

    const editor = CodeMirror.fromTextArea(textAreaRef.current, initOptions);
    editorRef.current = editor;
  }, []);

  useEffect(() => {
    // This will never happen, but useful for type checking.
    if (!editorRef.current) {
      return;
    }

    editorRef.current.setValue(value);
    onChange?.(editorRef.current);
  }, [value]);

  return (
    <Container>
      <textarea ref={textAreaRef} />
    </Container>
  );
}
