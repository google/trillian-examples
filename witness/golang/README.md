Go Witness
==============

The witness is an HTTP service that stores checkpoints it has seen from
different verifiable logs in a sqlite database.  This is a very lightweight way
to help detect or even prevent split-view attacks.

Once up and running, the witness provides three API endpoints (as defined in
[api/http.go](api/http.go)):
- `/witness/v0/logs` returns a list of all logs for which the witness is
  currently storing a checkpoint.
- `/witness/v0/logs/<logid>/update` acts to update the checkpoint stored for 
  `logid`.
- `/witness/v0/logs/<logid>/checkpoint` returns the latest checkpoint for
  `logid`, signed by the witness.

Running the witness
--------------------

Running the witness is as simple as running `go run main.go` (where `main.go`
can be found in the `cmd/witness` directory), with the following flags:
- `listen`, which specifies the address and port to listen on.
- `db_file`, which specifies the desired location of the sqlite database.  The
  use of sqlite limits the scalability and reliability of the witness (because
  this is a local file), so if that is required a different database backend
  would be needed.
- `config_file`, which specifies configuration information for the logs.  An
  sample configuration file is at `cmd/witness/example.conf`, and in general it
  is necessary to specify the following fields for each log:
    - `logID`, which is the identifier for the log.
    - `pubKey`, which is the public key of the log.  Given the current reliance on the Go [note format](https://pkg.go.dev/golang.org/x/exp/sumdb@v0.0.2/internal/note), the witness supports only Ed25519 signatures.
    - `hashStrategy`, which is the way in which recursive hashes are formed in the verifiable log.  The witness currently supports only `default` for this field.
    - `useCompact`, which is a boolean indicating if the log proves consistency via "regular" consistency proofs, in which case the witness stores only the latest checkpoint in its database, or via compact ranges, in which case the witness stores the latest checkpoint and compact range.
- `private_key`, which specifies the private signing key of the witness.  Again,
  the witness currently supports only Ed25519 signatures.
