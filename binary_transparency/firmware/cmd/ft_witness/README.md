# STH Witness


## Introduction

Firmware Transparency Demo steps outlined under [Firmware Transparency](firmware) provides a robust facility against verifying that the supplied firmware update by a FW Vendor is visible via a public log server. The supplied proofs from the log server and its verification locally by clients give sufficient confidence that the FW is verifiable, and the FT log tree is consistent, as presented to individual clients by the log server.

However, the current mechanism does not ensure that the view presented to all the FT clients is a consistent view of the log, i. e. the server is not presenting a forked or a different view to a subset of clients. In the transparency world, this is known as split-view attack.

## STH Witness Feature

To ensure that all clients have consistent view of the log, FT demo has been enhanced to run an independent witness server. The role of witness server is to maintain a reference log checkpoint, which can be requested by clients for verification.  To achieve this, a witness would periodically fetch the latest log checkpoint from the log server. The latest checkpoint is compared with the already saved reference log checkpoint with the witness. If different (i. e. the log server has new entries) the witness would then check the consistency of the fetched checkpoint from its previously stored reference log checkpoint to ensure that the newly received checkpoint is consistent. Upon verification the witness overwrites its own reference log checkpoint, with the newly received checkpoint. The reference log checkpoint is now made available to external clients. Reference log checkpoint of witness server is now referred as “Witness checkpoint”.

Any device client receiving the Firmware Update, from firmware vendor can perform additional verification steps using the Witness Server:
* Device client fetches the Witness checkpoint from Witness Server
* Device client receives its Update Log checkpoint (in firmware update bundle)
* Device client fetches consistency proof (CProof) between Update log checkpoint and Witness checkpoint from the log server
* Using CProof, client can now locally verify the consistency between the Update log checkpoint and Witness Checkpoint, to ensure that it is not presented a forked view of the FT log tree.

## Example Workflow

First run the Witness Server to periodically poll the log server to fetch its witness checkpoint:

```bash
# Run the witness server using the below command
go run ./cmd/ft_witness/ --logtostderr -v=2 --ws_db_file="/tmp/ws_ft.db"

```

The witness server will now be running at localhost:8020. We can point the flash tool at this server to perform additional checks by passing --witness_url=http://localhost:8020 when flashing to the device, e.g:

```bash
# Use the flash tool command with the witness server argument as below
* `go run ./cmd/flash_tool/ --logtostderr --update_file=/tmp/update.ota --device_storage=/tmp/dummy_device --device=dummy --witness_url=http://localhost:8020`
```