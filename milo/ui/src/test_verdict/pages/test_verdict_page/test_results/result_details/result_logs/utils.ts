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

const supportedContentTypes: { [key: string]: boolean } = {
  'application/octet-stream': true,
  'application/x-gzip': true,
};

// This function (and all the code in this file) is copied
// from ResultDB as we need to assert whether or not
// we can view the file in the UI as well.
export function isLogSupportedArtifact(
  artifactId: string,
  contentType: string,
): boolean {
  return isSupportedContentType(contentType) && !isNonTextFile(artifactId);
}

function isSupportedContentType(contentType: string): boolean {
  // Handle empty contentType (same as Go)
  if (contentType === '') {
    return true;
  }

  // Check if contentType is directly in the supported list
  if (supportedContentTypes[contentType]) {
    return true;
  }

  // Check for "text" prefix in the contentType (mimics Go's behavior)
  return contentType.startsWith('text');
}

function isNonTextFile(filePath: string): boolean {
  const ext = getFileExtension(filePath);

  // In the browser, we use a lookup table for MIME types
  const mimeType = getMimeTypeByExtension(ext);

  if (ext !== '') {
    // Check if MIME type is available and doesn't start with "text/"
    return mimeType !== undefined && !mimeType.startsWith('text/');
  } else {
    // If no extension, we assume it's not a non-text file
    return false;
  }
}

// Helper functions to handle file extensions and MIME types in the browser
function getFileExtension(filePath: string): string {
  // Extract extension (e.g., ".txt" from "document.txt")
  return filePath.split('.').pop() || '';
}

// Define a partial lookup for common MIME types
const textMimeTypeLookup: { [key: string]: string } = {
  txt: 'text/plain',
  html: 'text/html',
  htm: 'text/html',
  css: 'text/css',
  js: 'text/javascript',
  csv: 'text/csv',
  md: 'text/markdown',
  yaml: 'text/yaml',
  yml: 'text/yaml',
  heic: 'image/heic',
};

function getMimeTypeByExtension(ext: string): string | undefined {
  // Lookup the MIME type based on the extension
  return textMimeTypeLookup[ext.toLowerCase()]; // Case-insensitive lookup
}
