#!/usr/bin/env python3
# Copyright 2021 The LUCI Authors.
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

import argparse
import datetime
import logging
import sys
import hashlib
import os
import contextlib
import json
import subprocess
import re


KNOWN_REPOS = {
    'public': 'https://chromium.googlesource.com/playground/gerrit-cq',
    'internal': 'https://chrome-internal.googlesource.com/playground/cq',
}


def parse_args():
  p = argparse.ArgumentParser()
  p.add_argument('-n', '--number', type=int, required=True,
      help='CLs number')
  p.add_argument('--cq', type=int, required=True,
      help='+1 or +2 to vote on CQ label')
  p.add_argument('-w', '--work-dir', dest='workdir', required=True,
      help='Path where repos are checked out and results are stored')
  p.add_argument('--id',
      help='ID of this test. Defaults to current timestamp')
  p.add_argument('--description', default='UNSPECIFIED',
      help='Description of this test, should fit on one line')
  p.add_argument('--repo', default='internal',
      help='either "internal" or "public"')
  opts = p.parse_args()
  if not opts.id:
    opts.id = datetime.datetime.now().strftime('%Y%m%dT%H%M%S')
  elif ' ' in opts.id:
    p.error('spaces not allowed in --id')
  if opts.repo not in KNOWN_REPOS:
    p.error('only %s are supported for --repo' % sorted(KNOWN_REPOS))
  return opts


def main():
  opts = parse_args()
  logging.basicConfig(level=logging.DEBUG)
  ensure_work_dir(opts)
  checkout_repos(opts)
  cls = create_cls(opts)


def ensure_work_dir(opts):
  if not os.path.exists(opts.workdir):
    logging.info('Creating %r workdir', opts.workdir)
    os.makedirs(opts.workdir)
    return
  if os.path.isdir(opts.workdir):
    return
  raise Exception('%s is not a dir' % opts.workdir)


def checkout_repos(opts):
  for repo, origin in KNOWN_REPOS.items():
    if os.path.exists(os.path.join(opts.workdir, repo)):
      continue
    git('clone', origin, repo, cwd=opts.workdir)


def create_cls(opts):
  cwd = os.path.join(opts.workdir, opts.repo)
  git('freeze', cwd=cwd)  # just in case
  git('fetch', 'origin', cwd=cwd)  # avoid conflicts

  hash_prefix = hashlib.sha1(opts.id.encode('utf8')).hexdigest()[:-4]
  git('new-branch', '%s-%s' % (opts.id, hash_prefix), cwd=cwd)

  parent = os.path.join('cl-stack-tests', opts.id)
  logging.info('Creating %d commits in %s %s', opts.number, opts.repo, parent)
  os.makedirs(os.path.join(cwd, parent))
  for i in range(opts.number):
    gen_commit(opts, n=i+1, rel_parent_dir=parent, hash_prefix=hash_prefix, cwd=cwd)

  rev = git('rev-parse', 'HEAD', cwd=cwd)

  logging.info('Creating %d CLs by pushing %s to Gerrit', opts.number, rev[:8])
  git('push', 'origin', 'HEAD:refs/for/refs/heads/main',
      '-o', 'l=Code-Review+1',
      '-o', 'l=Commit-Queue+2',
      '-o', 'hashtag=cv-test',
      '-o', 'hashtag=%s' % opts.id,
      cwd=cwd)

def gen_commit(opts, n, rel_parent_dir, hash_prefix, cwd):
  rel_fpath = os.path.join(rel_parent_dir, '%03d' % n)
  # Full sha1 hash is 40 chars long.
  suffix_tmpl = '%%0%dd' % (40-len(hash_prefix),)
  change_id = hash_prefix + (suffix_tmpl % (n,))
  with open(os.path.join(cwd, rel_fpath), 'w') as f:
    f.write('Commit #%d\n' % n)
  git('add', rel_fpath, cwd=cwd)
  msg_lines = [
      'Commit #%d' % n,
      '',
      'Test-Description: %s' % opts.description,
      'Test-Id: %s' % opts.id,
      'Change-Id: I%s' % change_id,
  ]
  msg = '\n'.join(msg_lines)
  git('commit', '-m', msg, cwd=cwd)


def git(*args, cwd=None):
  out = subprocess.check_output(['git'] + list(args), cwd=cwd)
  return out.strip()


if __name__ == '__main__':
  main()
  sys.exit(0)
