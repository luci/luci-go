import time
import fcntl
import logging
import os
import struct

def _try_shared_file_lock(lockfile_fd):
  try:
    # fcntl.flock(lockfile_fd.fileno(), fcntl.LOCK_SH | fcntl.LOCK_NB)
    #fcntl.fcntl(lockfile_fd.fileno(), fcntl.F_SETFL, os.O_NONBLOCK | os.O_RDONLY)

    # fcntl.flock(lockfile_fd.fileno(), fcntl.LOCK_SH | fcntl.LOCK_NB)
    # fcntl.flock(lockfile_fd.fileno(), fcntl.LOCK_SH)
    fcntl.lockf(lockfile_fd.fileno(), fcntl.LOCK_SH | fcntl.LOCK_NB)

    # lockdata = struct.pack('hhllhh', fcntl.F_RDLCK, 0, 0, 0, 0, 0)
    # fcntl.fcntl(lockfile_fd.fileno(), fcntl.F_SETLK, lockdata)
    # lockdata = struct.pack('hhllhh', fcntl.F_SHLCK, 0, 0, 0, 0, 0)
    # fcntl.fcntl(lockfile_fd.fileno(), fcntl.F_SETLKW, lockdata)

    return True
  except IOError:
    # Something happened where the file lock couldn't be acquired (e.g the
    # lock is already held by another process).
    print('IO', IOError)
    return False

def _file_unlock(lockfile_fd):
  fcntl.lockf(lockfile_fd.fileno(), fcntl.LOCK_UN)
  print('unlockeddddd')


t = 30
print("This is the fake testing, locking the mmutex.lock for %d seconds", t)

MMUTEX_LOCK_FILE_PATH = "/usr/local/google/home/wenbinzhang/mmutex/mmutex.lock"
_MMUTEX_LOCKFILE_FD = open(MMUTEX_LOCK_FILE_PATH)
success = _try_shared_file_lock(_MMUTEX_LOCKFILE_FD)

if not success:
  print('[mmutex] acquire_maintenance_mutex failure (acquisition)')
  _MMUTEX_LOCKFILE_FD.close()
  _MMUTEX_LOCKFILE_FD = None
  quit()
else:
  print('[mmutex] acquire_maintenance_mutex success')

print('start sleeping')
time.sleep(t)
print('wake up')

if _MMUTEX_LOCKFILE_FD:
  print('[mmutex] Releasing maintenance mutex')
  _file_unlock(_MMUTEX_LOCKFILE_FD)
  _MMUTEX_LOCKFILE_FD.close()
  _MMUTEX_LOCKFILE_FD = None
else:
  print('[mmutex] Not releasing maintenance mutex; not held')
