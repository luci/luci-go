#!/usr/bin/env python3

# Copyright 2025 The LUCI Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# This script doesn't take any options and runs golangci-lint correctly* over the LUCI tree.
# * Terms and Conditions apply.
#
# Useful files when hacking (use "gf" to visit them in (neo)vim)
# ./scripts/golangci/library.yaml
# ./scripts/golangci/project.yaml

import os
import pathlib
import subprocess
import sys

def golangci_lint_fix():
    """
    Runs golangci-lint with the --fix flag on all directories containing .golangci* files.
    The script changes the current directory to the location of the script before running.
    """
    # Get the directory of the current script.
    script_dir = pathlib.Path(__file__).parent

    # Run the golangci-lint config updater so we don't forget.
    subprocess.run(script_dir / "scripts/regen_golangci_config.py", cwd=script_dir)

    # Find directories containing .golangci* files
    dirs_to_lint = []
    for path in pathlib.Path(script_dir).rglob(".golangci*"):
      dirs_to_lint.append(os.path.abspath(path.parent))

    if not dirs_to_lint:
        raise RuntimeError("no directories found")

    # Run golangci-lint with --fix on the found directories
    for subdir in dirs_to_lint:
        args = ["golangci-lint", "run", "--fix", f"--path-prefix=./{os.path.basename(subdir)}", "./..."]
        print(f"Running {args} in {subdir}")
        try:
            subprocess.run(
                args,
                check=True,
                cwd=subdir,
            )
        except subprocess.CalledProcessError:
            print("golangci-lint exited abnormally", file=sys.stderr)
            return 1

    print("golangci-lint run completed successfully.", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(golangci_lint_fix())
