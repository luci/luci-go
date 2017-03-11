# Copyright 2017 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

import argparse
import json
import sys

# Also, import our two VirtualEnv packages.
import pants
import shirt


def main(args):
  parser = argparse.ArgumentParser()
  parser.add_argument('--json-output',
      help='JSON output path.')
  opts = parser.parse_args(args)

  manifest = {
      'interpreter': sys.executable,
      'pants': pants.__file__,
      'shirt': shirt.__file__,
  }

  if opts.json_output:
    with open(opts.json_output, 'w') as fd:
      json.dump(manifest, fd)
  return 0

if __name__ == '__main__':
  sys.exit(main(sys.argv[1:]))
