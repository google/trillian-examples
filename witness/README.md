# Log Witnessing

## Overview

Witnessing is an important part of log ecosystems that prevents logs from
successfully accomplishing a _split-view attack_. A split-view attack is
one in which the log presents different populations with different views.
Each of these views may be append-only, and thus clients checking consistency
proofs would be satisfied, yet the log is not fulfilling its claim of providing
a _globally consistent_ list.

For each witnessed log, a Witness cosigns log _checkpoints_ only if it
can verify that it is consistent with the last checkpoint it cosigned, i.e.
that this new checkpoint represents only new values being appended to the log.
Clients that acquire checkpoints signed by multiple independent witnesses can
have more confidence that they are seeing a truly globally consistent log.

## Roles

We consider witnessing to be composed of three primary _roles_:
  1. Feeder: provides checkpoints and consistency proofs to the witness
  2. Witness: maintains and cosigns a checkpoint for each log, and updates
     this state to a new checkpoint only when convinced that the delta is only
     appending new leaves
  3. Distributor: makes cosigned checkpoints available to clients

Note that this does not mean there needs to be three separate actors playing these roles.
As a counterpoint, a common situation is to have the actor playing the witness role
also polling the log for checkpoints and proofs (i.e. playing the feeder role), and
making cosigned checkpoints available via some API (distributor).

## Generic Witness Implementation

This repository provides a witness that works for any log that uses the
[log checkpoint format](https://github.com/transparency-dev/formats/tree/main/log).
Each log requires a custom feeder to provide the binding between the log and the
witness. Such feeders are intentionally not provided here in order to maintain a
logical separation, and to avoid this package acquiring vast numbers of code
dependencies. That said, [Feeders](#feeders) has links to known implementations.

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

Specific feeders are listed below, however potential witness operators are advised to
simply deploy the [Omniwitness](golang/omniwitness/monolith) which contains all of these
feeders, unless there is a compelling reason to limit the witnessed logs.

* [Go SumDB](https://github.com/google/trillian-examples/tree/master/sumdbaudit/witness)
* [Serverless](https://github.com/google/trillian-examples/tree/master/serverless/cmd/feeder)
* [Pixel BT](https://github.com/google/trillian-examples/tree/master/feeder/cmd/pixel_bt_feeder)
* [Rekor/SigStore](https://github.com/google/trillian-examples/tree/master/feeder/cmd/rekor_feeder)

