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
import { Theme } from '@emotion/react';
import styled, { Interpolation } from '@emotion/styled';
import * as CodeMirror from 'codemirror';
import { useEffect, useRef } from 'react';

const Container = styled.div`
  display: block;
  border-radius: 4px;
  border: 1px solid var(--divider-color);
  overflow: auto;

  & .CodeMirror {
    height: auto;
    font-size: 12px;
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
  readonly onInit?: (editor: CodeMirror.Editor) => void;
  readonly css?: Interpolation<Theme>;
  readonly className?: string;
}

// We cannot use @uiw/react-codemirror@4 because it uses codemirror v6, which
// no longer supports viewportMargin: Infinity, which is required to support
// searching content hidden behind a scrollbar.
// We cannot use @uiw/react-codemirror@3 because it yields various react-dom
// validation errors with the current version of React.
// And neither version offers a good way to attach fold/unfold event listeners
// BEFORE any content is rendered.
export function CodeMirrorEditor({
  value,
  initOptions,
  onInit,
  css,
  className,
}: CodeMirrorEditorProps) {
  const textAreaRef = useRef<HTMLTextAreaElement>(null);
  const editorRef = useRef<CodeMirror.EditorFromTextArea | null>(null);

  // Wrap them in refs so eslint doesn't complain about missing dependencies in
  // `useEffect` hooks.
  const firstInitOptions = useRef(initOptions);
  const firstOnInit = useRef(onInit);

  useEffect(() => {
    // This will never happen, but useful for type narrowing.
    if (!textAreaRef.current) {
      return;
    }

    if (editorRef.current) {
      return;
    }

    const editor = CodeMirror.fromTextArea(
      textAreaRef.current,
      firstInitOptions.current,
    );
    editorRef.current = editor;
    firstOnInit.current?.(editorRef.current);
  }, []);

  useEffect(() => {
    // This will never happen, but useful for type narrowing.
    if (!editorRef.current) {
      return;
    }

    editorRef.current.setValue(value);

    // Somehow codemirror does not display the content when the editor is
    // initialized very early (it's likely that codemirror has some internal
    // asynchronous initialization steps).
    // Force refresh AFTER all functions in the present queue are executed to
    // to ensure the content are rendered properly.
    setTimeout(() => editorRef.current?.refresh());
  }, [value]);

  return (
    <Container className={className} css={css}>
      <textarea ref={textAreaRef} />
    </Container>
  );
}
