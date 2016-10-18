#!/usr/bin/env python
# Copyright 2016 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

"""Adhoc test to verify cipd can replace running executables on Windows.

Assumes active Go environment. Will rebuild cipd client before executing
the test.

Meant to be run manually.
"""

import contextlib
import os
import shutil
import subprocess
import tempfile


TESTING_DIR = os.path.dirname(os.path.abspath(__file__))
TEMP_DIR = os.path.join(TESTING_DIR, '.tmp_dir')


SLEEPER_GO=r"""
package main

import "time"

func main() {
  time.Sleep(30 * time.Second)
}
"""


@contextlib.contextmanager
def temp_dir(work_dir):
  tmp = tempfile.mkdtemp(dir=work_dir)
  try:
    yield tmp
  finally:
    shutil.rmtree(tmp)


def build_sleeper(work_dir, out):
  with temp_dir(work_dir) as tmp:
    with open(os.path.join(tmp, 'main.go'), 'wt') as f:
      f.write(SLEEPER_GO)
    subprocess.check_call(['go', 'build', '-o', out, '.'], cwd=tmp)


def build_cipd(out):
  subprocess.check_call([
    'go', 'build',
    '-o', out,
    'github.com/luci/luci-go/cipd/client/cmd/cipd',
  ])


def list_dir(path):
  print 'File tree:'
  for root, subdirs, files in os.walk(path):
    for p in subdirs:
      print 'D ' + os.path.join(root, p)[len(path)+1:]
    for p in files:
      print 'F ' + os.path.join(root, p)[len(path)+1:]


def main():
  if not os.path.exists(TEMP_DIR):
    os.mkdir(TEMP_DIR)

  print 'Building CIPD client'
  cipd_exe = os.path.join(TEMP_DIR, 'cipd.exe')
  build_cipd(cipd_exe)

  print 'Building test executable'
  sleeper_exe = os.path.join(TEMP_DIR, 'sleeper.exe')
  build_sleeper(TEMP_DIR, sleeper_exe)

  sleeper_cipd = os.path.join(TEMP_DIR, 'sleeper.cipd')
  with temp_dir(TEMP_DIR) as tmp:
    tmp_sleeper_exe = os.path.join(tmp, 'sleeper.exe')

    print 'Packaging the test executable into a cipd package'
    shutil.copy(sleeper_exe, tmp_sleeper_exe)
    subprocess.check_call([
      cipd_exe, 'pkg-build',
      '-in', tmp,
      '-name', 'adhoc-testing',
      '-out', sleeper_cipd,
    ])

    print 'Starting the test executable and attempt to update it by cipd'
    print 'Exe: %s' % tmp_sleeper_exe
    sleeper_proc = subprocess.Popen([tmp_sleeper_exe])
    try:
      subprocess.check_call([
        cipd_exe, 'pkg-deploy',
        '-root', tmp,
        sleeper_cipd,
      ])
      # This shows that exe is updated, and the old exe (still running) is now
      # in trash (<root>/.cipd/trash/).
      list_dir(tmp)
    finally:
      sleeper_proc.terminate()
      sleeper_proc.wait()

    # Do it again. This time it should remove all the trash.
    subprocess.check_call([
      cipd_exe, 'pkg-deploy',
      '-root', tmp,
      sleeper_cipd,
    ])
    # This shows that the trash directory is empty now.
    list_dir(tmp)


if __name__ == '__main__':
  main()
