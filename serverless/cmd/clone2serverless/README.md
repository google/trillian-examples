# clone2serverless

This tool create a [tile-based log](https://research.swtch.com/tlog#tiling_a_log) on the local filesystem.
The log data is read from a MySQL database that has been populated using one of the [clone tools](../../../clone/cmd/).

## Setup

It is strongly recommended to create a virtual filesystem for any sizeable log.
The tile-based log will create `O(N)` files & directories, which can quickly exhaust the inodes for an `ext4` filesystem provisioned for "normal" desktop/server usage.

The instructions below demonstrate one way to create a [btrfs](https://btrfs.readthedocs.io/en/latest/) filesystem in a file.
Creating the filesystem in a file is a convenient way to experiment with a new filesystem:
 - set up without formatting or partitioning existing disks
 - minimize the risk of inode exhaustion on the parent filesystem
 - tear down by deleting the file

```shell
export LOGID=sumdb
# Create a 32 GB file to hold the filesystem
sudo truncate -s32G /media/${LOGID}.btrfs.img
# Provision a btrfs filesystem inside the new file
sudo mkfs.btrfs /media/${LOGID}.btrfs.img
# Create a mount point to attach the new filesystem
sudo mkdir /mnt/${LOGID}_tlog
# Mount the filesystem using a loopback interface
sudo mount -t auto -o loop /media/${LOGID}.btrfs.img /mnt/${LOGID}_tlog
# Make the mount point owned by the normal user
sudo chown $USER /mnt/${LOGID}_tlog
```

See [Tear Down](#tear-down) for instructions to free up space afterwards.

## Running

The following command will create a tlog from the [sumdb](../../../clone/cmd/sumdbclone/) cloned database into a directory called `fs` within a btrfs filesystem mounted at `/mnt/sumdb_tlog`:

```shell
go run ./serverless/cmd/clone2serverless \
  --mysql_uri='sumdb:letmein@tcp(localhost:3366)/sumdb' \
  --output_root=/mnt/sumdb_tlog/fs \
  --log_vkey=sum.golang.org+033de0ae+Ac4zctda0e5eza+HJyk9SxEdh+s3Ux18htTTAD8OuAn8 \
  --log_origin="go.sum database tree" \
  --alsologtostderr --v=1
```

## Verifying

As a quick check to make sure that the tlog was generated successfully, we can use the serverless client tool to check an inclusion proof:

```shell
echo sum.golang.org+033de0ae+Ac4zctda0e5eza+HJyk9SxEdh+s3Ux18htTTAD8OuAn8 > /tmp/sumdbvkey
go run ./serverless/cmd/client \
  --log_public_key=/tmp/sumdbvkey \
  --origin="go.sum database tree" \
  --log_url=file:////mnt/sumdb_tlog/fs/ \
  inclusion /mnt/sumdb_tlog/fs/seq/00/00/c9/5d/12 0xc95d12
```

## Tear Down

To delete the tlog and free up the resources on disk:

```shell
export LOGID=sumdb
sudo umount /mnt/${LOGID}_tlog
sudo rm /media/${LOGID}.btrfs.img
```
