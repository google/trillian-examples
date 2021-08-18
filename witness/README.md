# Log Witnessing

## Overview

Witnessing is an important part of log ecosystems that prevents logs from
successfully accomplishing a _split-view attack_. A split-view attack is
one in which the log prevents different populations with different views.
Each of these views is append-only, and thus all clients checking consistency
proofs are satisfied, yet the log is not fulfilling its claim of providing a
_globally consistent_ list.

For each witnessed log, a Witness will cosign log _checkpoints_ only if it
can verify that it is consistent with the last cosigned checkpoint, i.e. that
this new checkpoint represents only new values being appended to the log.
Clients that acquire checkpoints signed by multiple independent witnesses can
have more confidence that they are seeing a truly globally consistent log.

## Roles

We consider witnessing to be composed of three primary _roles_:
  1. Feeder: acquires checkpoints and consistency proofs, and provides these
     to the witness
  2. Witness: maintains and cosigns a checkpoint for each log, and updates
     this state to a new checkpoint only when convinced that the delta is only
     appending new leaves
  3. Distributor: makes cosigned checkpoints available to clients

Note that this does not mean there needs to be 3 separate actors playing these roles.
As a counterpoint, a common situation is to have the actor playing the witness role
also polling the log for checkpoints and proofs (i.e. playing the feeder role), and
making cosigned checkpoints available via some API (distributor).

## Generic Witness Implementation

This repository provides a witness that will work for any log that uses the
[format defined in this
repository](https://github.com/google/trillian-examples/tree/master/formats/log).
Each log will require a custom feeder implementation to provide the binding
between the log and the witness. Such feeders are intentionally not provided here
in order to maintain a logical separation, and to avoid this package acquiring
vast numbers of code dependencies. That said, [Feeders](#feeders) has links to known
feeder implementations.

In terms of checking consistency, witnesses can do so in the following two 
(non-exhaustive) ways:
- Getting a consistency proof as defined in [RFC
  6962](https://datatracker.ietf.org/doc/html/rfc6962#section-2.1.2), assuming 
  all they have is a log root.
- Getting a [compact range](https://arxiv.org/pdf/2011.04551.pdf), assuming they 
  maintain a compact range covering the log up to the point at which 
  they've already ensured its consistency.

We consider an interface for a witness defined as follows:

- `GetCheckpoint(logID)`: returns a checkpoint for this log, co-signed by the
  witness.  Ideally, but not necessarily, this is the latest checkpoint.
- `Update(logID, chkpt, pf)`: checks the signature in the checkpoint and verifies 
  its consistency with its latest checkpoint from this log signer according to 
  the proof.  If all checks pass then add the checkpoint to the list maintained
  for this log. If the witness has no previous checkpoint for this log then this
  checkpoint is considered trust-on-first-use.  Either way, the witness
  returns the latest checkpoint it has stored for this log.

## Index

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

## Feeders

* [Go SumDB](https://github.com/google/trillian-examples/tree/master/sumdbaudit/witness)
* [Serverless Logs](https://github.com/google/trillian-examples/tree/master/serverless/cmd/feeder)