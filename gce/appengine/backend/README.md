# GCE Backend

A backend package for the GCE GAE app. Comprised of independent, idempotent
cron jobs which trigger independent, idempotent task queues which attempt to
move the real-world state of GCE instances closer to the configured state of GCE
instances. The cron jobs and task queues are fault tolerant-- failures do not
generally cause inconsistent state, allowing the task queues to be triggered
again later by the cron jobs. This means transient failures such as datastore or
network outages and insufficient permissions or quota only cause failures in the
backend package as long as they remain unresolved. Once the issues are resolved,
the backend package should recover without intervention.

## Terminology

### Config

A Config is a datastore entity representing a configured type of VM. Creation of
Configs is outside the scope of the backend package. Configs are mutable and may
be created, updated, or even deleted at any time and the backend package will
react accordingly.

### VM

A VM is a datastore entity representing a single configured VM, derived from a
Config. [expandConfig](#expandConfig) is responsible for the derivation. VMs
are mutable, but should only be modified by the backend package. To make changes
to a VM, modify its corresponding Config and the backend package will propagate
the changes. The Config:VM mapping is 1:n.

### GCE Instance

A GCE instance is a live virtual machine running in Google Compute Engine. An
instance is created from a VM by [createInstance](#createInstance). Instances
are immutable. Changes made to a VM will only be reflected when creating a new
instance. The VM:instance mapping is 1:1.

### Swarming Bot

A Swarming bot is the Swarming server's view of a connected instance. Instances
automatically register themselves as bots of a particular Swarming server
outside the scope of the backend package. Bots may freely be terminated or
deleted from the Swarming server and the backend package will react accordingly.
The instance:bot mapping is 1:1.

### Deadline

The deadline is how long an instance may live for. An instance's deadline is
derived from the lifetime in the Config and the instance creation time. Once the
deadline is up, the backend package will attempt to replace the instance after
it finishes its current Swarming workload. Replacing the instance is how changes
to VMs are picked up, since instances are immutable.

### Drained

A drained VM is one scheduled for deletion because the Config has been altered
to have its number of VMs decreased. A drained VM will be deleted once its
corresponding instance has been deleted. A drained Config is one scheduled for
deletion by some external factor. All VMs of a drained Config will be drained. A
drained Config will be deleted once its corresponding VMs have been deleted.

## Cron Jobs

All cron jobs operate on multiple entities, triggering task queues which operate
on a particular entity. All cron jobs are idempotent.

### expandConfigsAsync

expandConfigsAsync iterates over all Configs and triggers
[expandConfig](#expandConfig) for each.

### createInstancesAsync

createInstancesAsync iterates over all VMs which have no corresponding instance
and triggers [createInstance](#createInstance) for each.

### manageBotsAsync

manageBotsAsync iterates over all VMs which do have a corresponding instance and
triggers [manageBot](#manageBot) for each.

## Task Queues

All task queues are triggered with a particular entity to process. All task
queues are idempotent.

### expandConfig

expandConfig receives a single Config to expand. It checks how many VMs the
Config declares and triggers [createVM](#createVM) for each.

### createVM

createVM receives a single VM to create. It creates the VM if it doesn't exist.

### createInstance

createInstance receives a single VM to create an instance for and attempts to
idempotently create it. Instance creation in GCE is asynchronous, so the backend
package calls createInstance repeatedly until it's detected as created and then
records it. Creation is completed if already started for a [drained](#drained)
VM, but new creation tasks in GCE are not started for drained VMs.

### manageBot

manageBot receives a single VM to manage a bot for. First checks if the Config
referenced by the VM no longer exists or no longer references the given VM and
[drains](#drain) the VM if it isn't already. Next, watches the Swarming server
for changes in the bot's state and reacts accordingly. If Swarming reports that
the bot has died or been deleted or terminated, triggers
[destroyInstance](#destroyInstance). If the VM's deadline has been exceeded or
the VM is [drained](#drained), triggers [terminateBot](#terminateBot).

### destroyInstance

destroyInstance receives a single VM to destroy the created instance for and
attempts to idempotently destroy it. Instance deletion in GCE is asynchronous,
so the backend package calls destroyInstance repeatedly until it's detected as
destroyed and then triggers [deleteBot](#deleteBot).

### deleteBot

deleteBot receives a single VM entity to delete the bot for. Bot deletion in
Swarming is synchronous, so this action is recorded immediately, which deletes
the VM.

### terminateBot

terminateBot receives a single VM to terminate the bot for and attempts to
terminate it. Termination in Swarming is asynchronous, so the backend package
calls [manageBot](#manageBot) repeatedly until it's detected as terminated.
