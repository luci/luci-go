// Copyright 2022 The LUCI Authors.
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

import { useState, useRef, useMemo } from 'react';
import { useParams, useSearchParams } from 'react-router-dom';

import AceEditor from 'react-ace';
import 'ace-builds/src-noconflict/mode-json';
import 'ace-builds/src-noconflict/theme-tomorrow';
import 'ace-builds/src-noconflict/ext-language_tools';

import { useGlobals } from '../context/globals';
import { AuthMethod, AuthSelector } from '../components/auth_selector';


const Method = () => {
  const { serviceName, methodName } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const { descriptors, oauthClient } = useGlobals();
  const [authMethod, setAuthMethod] = useState(AuthMethod.OAuth);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<Error|null>(null);

  const requestEditor = useRef<AceEditor>(null);
  const responseEditor = useRef<AceEditor>(null);

  // Initial request body can be passed via `request` query parameter.
  // Pretty-print it if it is a valid JSON. Memo this to avoid reparsing
  // potentially large JSON all the time.
  let initialRequest = searchParams.get('request') || '{}';
  initialRequest = useMemo(() => {
    try {
      return JSON.stringify(JSON.parse(initialRequest), null, 2);
    } catch {
      return initialRequest;
    }
  }, [initialRequest]);

  // Find the method descriptor. It will be used for auto-completion and for
  // actually invoking the method.
  const svc = descriptors.service(serviceName ?? 'unknown');
  if (svc === undefined) {
    return <p>No such service</p>;
  }
  const method = svc.method(methodName ?? 'unknown');
  if (method === undefined) {
    return <p>No such method</p>;
  }

  const invokeMethod = () => {
    if (!requestEditor.current || !responseEditor.current) {
      return;
    }
    const reqEditor = requestEditor.current.editor;
    const resEditor = responseEditor.current.editor;

    // Default to an empty request.
    let requestBody = reqEditor.getValue().trim();
    if (!requestBody) {
      requestBody = '{}';
      reqEditor.setValue(requestBody, -1);
    }

    // The request must be a valid JSON. Verify locally.
    let parsedReq: object;
    try {
      parsedReq = JSON.parse(requestBody);
    } catch (err) {
      if (err instanceof Error) {
        setError(err);
      } else {
        setError(new Error(`${err}`));
      }
      return;
    }

    // Update the current location to allow copy-pasting this request via URI.
    // Use compact request serialization (strip spaces etc).
    setSearchParams((params) => {
      const compact = JSON.stringify(parsedReq);
      if (compact != '{}') {
        params.set('request', compact);
      } else {
        params.delete('request');
      }
      return params;
    }, {replace: true});

    // Deactivate the UI while the request is running.
    reqEditor.setReadOnly(true);
    setRunning(true);

    // Clear the old response and error, if any.
    resEditor.setValue('');
    setError(null);

    // Grabs the authentication header and invokes the method.
    const authAndInvoke = async () => {
      let authorization = '';
      if (authMethod == AuthMethod.OAuth) {
        authorization = `Bearer ${await oauthClient.accessToken()}`;
      }
      return await method.invoke(requestBody, authorization);
    };
    authAndInvoke()
        .then((response) => resEditor.setValue(response, -1))
        .catch((error) => setError(error))
        .finally(() => {
          // Reactive the UI.
          reqEditor.setReadOnly(false);
          setRunning(false);
        });
  };

  return (
    <>
      <p>A method {methodName} of {serviceName}</p>
      <p>{method.help}</p>

      <div>
        <h4>Request:</h4>
        <AceEditor
          ref={requestEditor}
          mode='json'
          defaultValue={initialRequest}
          theme='tomorrow'
          name='request-editor'
          width='800px'
          height='200px'
          setOptions={{
            enableBasicAutocompletion: true,
            useWorker: false,
          }}
        />
      </div>

      <AuthSelector
        selected={authMethod}
        onChange={setAuthMethod}
        oauthClientId={oauthClient.clientId}
        disabled={running}
      />

      {running && <p>Running</p>}
      {!running && <button type="button" onClick={invokeMethod}>Send</button>}

      {error && <p>Error: {error.message}</p>}

      <div>
        <h4>Response:</h4>
        <AceEditor
          ref={responseEditor}
          mode='json'
          theme='tomorrow'
          name='response-editor'
          width='800px'
          height='400px'
          setOptions={{
            readOnly: true,
            useWorker: false,
          }}
        />
      </div>
    </>
  );
};

export default Method;
