Witnessing
============

Witnessing is an important part of any transparent ecosystem, as it ensures that
misbehavior on the part of logs can be prevented or detected. This is done by
ensuring that the _checkpoints_ published by logs are _consistent_, which means 
the log is presenting a globally consistent view of its contents to everyone and
thus isn't carrying out a _split-view attack_.

In terms of checkpoints, we consider witness implementations that use the
[format defined in this
repository](https://github.com/google/trillian-examples/tree/master/formats/log).

In terms of checking consistency, witnesses can do so in the following two 
(non-exhaustive) ways:
- Getting a "classical" consistency proof, assuming all they have is a log root.
- Getting a compact range, assuming they maintain a compact range.

We consider an interface for a witness defined as follows:

- `GetCheckpoint(logID)`: returns a checkpoint for this log, co-signed by the
  witness.  Ideally, but not necessarily, this is the latest checkpoint.
- `Update(logID, chkpt, pf)`: checks the signature in the checkpoint and checks
  its consistency with its latest checkpoint from this log signer according to 
  the proof.  If all checks pass then add the checkpoint to the list maintained
  for this log. If the witness has no previous checkpoint for this log then this
  checkpoint is considered trust-on-first-use.  Either way, the witness
  returns the size of the latest checkpoint it has stored for this log.

Index
--------------------------

This directory contains witness implementations that provide the interface
defined above.  Currently it contains implementations in the following languages:

- [Go](golang) 
  This can either be run as a standalone application or integrated
  into a larger one (for example an entity that might want to do other,
  application-specific checks).  This witness supports both methods of checking
  consistency.
- [Solidity](ethereum) 
  This can be deployed as a smart contract on the Ethereum blockchain.  There are 
  two main contracts, each of which supports one of the consistency checks 
  described above.
