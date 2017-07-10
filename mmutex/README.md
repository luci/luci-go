Maintenance Mutex (mmutex)
==========================

mmutex is a command line tool that helps prevent maintenance tasks from running
during user tasks. The tool does this by way of a global lock file that users 
must acquire before running their tasks.

Clients can use this tool to request that their task be run with one of two 
types of access to the system:

  * **Exclusive access** guarantees that no other callers have any access
    exclusive or shared) to the resource while the specified command is run.
  * **Shared access** guarantees that only other callers with shared access
    will have access to the resource while the specified command is run.

In short, exclusive access guarantees a task is run alone, while shared access
tasks may be run alongside other shared access tasks.

TODO(charliea): Document actual command line usage here.
