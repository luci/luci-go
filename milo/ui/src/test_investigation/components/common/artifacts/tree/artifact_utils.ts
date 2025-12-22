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

/**
 * Determines the type of an artifact based on its ID (file name).
 * It primarily uses the file extension from the basename of the path.
 *
 * @param artifactId The ID of the artifact (e.g., "results/screenshot.png").
 * @returns A string representing the artifact type (e.g., "png"),
 * or "file" as a default for items without a clear extension.
 */
export function getArtifactType(artifactId: string): string {
  // Handle specific known names that don't have extensions.
  if (
    artifactId === 'image_diff' ||
    artifactId === 'expected_image' ||
    artifactId === 'actual_image'
  ) {
    return 'image';
  }

  // Get the filename from the full path.
  const fileName = artifactId.slice(artifactId.lastIndexOf('/') + 1);

  // Find the last dot in the filename.
  const lastDotIndex = fileName.lastIndexOf('.');

  // Ensure the dot is not the first character (for dotfiles like .bashrc)
  // and that there is an extension.
  if (lastDotIndex > 0) {
    const extension = fileName.slice(lastDotIndex + 1).toLowerCase();
    if (extension) {
      return extension;
    }
  }

  // Default type for files without a clear extension.
  return 'file';
}
