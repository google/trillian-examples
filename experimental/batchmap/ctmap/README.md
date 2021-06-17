# Verifiable Map for CT Logs

## Intro & Motivation

To verifiably find certificates in a log that have been issued for domains that a user controls, the user must inspect every certificate in the log.
The current alternative to this is to trust a non-verifiable log aggregator / search service.

A verifiable map built from a log maintains verifiability, while providing point lookup and proofs of non-inclusion.

## Status

This is a demo / proof of concept.

## Running

First you will need to clone a log locally using the `clone/cmd/ctclone` tool found in this repository.
Once a log has been downloaded and verified, a map can be built using a command such as:

```bash
go run -ldflags "-X google.golang.org/protobuf/reflect/protoregistry.conflictPolicy=warn" ./experimental/batchmap/ctmap/cmd/build --alsologtostderr --mysql_log_uri 'mirror:letmein@tcp(localhost)/google_xenon2022' --count 10000 --map_output_root_dir=/tmp/mapfun/
```

To run this in Dataflow, a GCP project must have been provisioned along with a Cloud Storage Bucket for the output (in the example below, this bucket is named `$GCP_PROJECT-xenon2022`).
Additionally, the `ctclone` tool should have run on a VM and populated a MySQL database with a known IP address.

To run the job on Dataflow in such a GCP project:

```bash
go run -ldflags "-X google.golang.org/protobuf/reflect/protoregistry.conflictPolicy=warn" ./experimental/batchmap/ctmap/cmd/build --alsologtostderr --v=2 --runner=dataflow --project=$GCP_PROJECT --region=us-central1 --staging_location=gs://$GCP_PROJECT-xenon2022/staging --mysql_log_uri 'mapper:letmein@tcp($MYSQL_IP)/googlexenon2022' --count 100 --map_output_root_dir=gs://$GCP_PROJECT-xenon2022/map/
```
