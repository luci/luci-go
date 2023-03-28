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

import { EditorConfiguration, ModeSpec } from 'codemirror';
import { useEffect, useRef } from 'react';
import { useLatest } from 'react-use';

import './connection_observer';
import { PropertyViewerConfigInstance } from '../store/user_config';
import { CodeMirrorEditor } from './code_mirror_editor';
import { ConnectionEvent, ConnectionObserverElement } from './connection_observer';

const LEFT_RIGHT_ARROW = '\u2194';

export interface PropertyViewerProps {
  readonly properties: { readonly [key: string]: unknown };
  readonly config: PropertyViewerConfigInstance;
}

export function PropertyViewer({ properties, config }: PropertyViewerProps) {
  const formattedValue = JSON.stringify(properties, undefined, 2);

  // Ensure the callbacks always refer to the latest values.
  const formattedValueLines = useLatest(formattedValue.split('\n'));
  const editorOptions = useRef<EditorConfiguration>({
    mode: { name: 'javascript', json: true } as ModeSpec<{ json: boolean }>,
    readOnly: true,
    matchBrackets: true,
    lineWrapping: true,
    foldGutter: true,
    lineNumbers: true,
    // Ensures all nodes are rendered therefore searchable.
    viewportMargin: Infinity,
    gutters: ['CodeMirror-linenumbers', 'CodeMirror-foldgutter'],
    foldOptions: {
      widget: (from) => {
        const line = formattedValueLines.current[from.line];
        // Not a root level property, ignore.
        if (!line.startsWith('  "')) {
          return LEFT_RIGHT_ARROW;
        }

        // Use <milo-connection-observer> to observer fold/unfold events.
        // We can't use a regular element with an onclick event handler because
        // code mirror clones the element and all event handlers are dropped.
        // We can't use the 'gutterClick' event because clicking on the widget
        // unfolds the region but doesn't trigger the 'gutterClick' event.
        const connectionObserver = document.createElement(
          'milo-connection-observer'
        ) as ConnectionObserverElement<string>;
        connectionObserver.setAttribute('event-type', 'folded-root-lvl-prop');
        connectionObserver.setAttribute('data', JSON.stringify(line));
        connectionObserver.innerHTML = `<span class="CodeMirror-foldmarker">${LEFT_RIGHT_ARROW}<span>`;
        return connectionObserver;
      },
    },
  });

  const containerRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!containerRef.current) {
      return;
    }

    const cb = (event: Event) => {
      const e = event as ConnectionEvent<string>;
      config.setFolded(e.detail.data, true);

      e.detail.addDisconnectedCB((data) => {
        config.setFolded(data, false);
      });
    };

    containerRef.current.addEventListener('folded-root-lvl-prop', cb);

    return () => {
      containerRef.current?.removeEventListener('folded-root-lvl-prop', cb);
    };
  }, []);

  const onChangeHandler = (editor: CodeMirror.Editor) => {
    formattedValueLines.current.forEach((line, lineIndex) => {
      if (config.isFolded(line)) {
        // This triggers folded-root-lvl-prop then this.toggleFold which
        // updates the timestamp in this.propLineFoldTime[line].
        // As a result, recently accessed this.propLineFoldTime[line] is
        // kept fresh.
        editor.foldCode(lineIndex);
      }
    });
  };

  return (
    <div ref={containerRef} css={{ minWidth: '600px', maxWidth: '1000px' }}>
      <CodeMirrorEditor value={formattedValue} initOptions={editorOptions.current} onChange={onChangeHandler} />
    </div>
  );
}
