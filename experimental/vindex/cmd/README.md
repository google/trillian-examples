# Verifiable Index from CloneDB

This binary takes a [Go SumDB clone](../../../clone/cmd/sumdbclone/) and generates a Verifiable Index from it, using the library provided in [transparency-dev/incubator/vindex](https://github.com/transparency-dev/incubator/tree/main/vindex).

## Running

This requires a populated [Go SumDB clone](../../../clone/cmd/sumdbclone/) that can be addressed.
Once this DB is populated, assuming that it was invoked via the `docker compose` setup, the following command will generate a verifiable index from it:

```shell
# Corresponding public key: example.com/sumdb/vindex+dc01ec4b+AZGQkIvW6u5vtO1DH9NtQUYSUW77q8temEuuXSOlg98+
OUTPUT_LOG_PRIVATE_KEY=PRIVATE+KEY+example.com/sumdb/vindex+dc01ec4b+Ac2oB+MNIOn72g69VfVEL2PdXhk3MASCO9XRhWDkHqW8 go run ./experimental/vindex/cmd \
  --logDSN="sumdb:letmein@tcp(127.0.0.1:33006)/sumdb"  \
  --storage_dir ~/vindex-sumdb/ \
  --v=1
```

Depending on the machine you run this on, and the size of the SumDB log, this could take several minutes before the Verifiable Index is available.

Once this is running, you can inspect the Output Log using:

```shell
go run github.com/mhutchinson/woodpecker@main \
  --custom_log_type=tiles \
  --custom_log_url=http://localhost:8088/outputlog/ \
  --custom_log_vkey=example.com/sumdb/vindex+dc01ec4b+AZGQkIvW6u5vtO1DH9NtQUYSUW77q8temEuuXSOlg98+
```

Use left/right cursor to browse, and `q` to quit.

This log is processed into a verifiable map which can be looked up using the following command:

```shell
go run github.com/transparency-dev/incubator/vindex/cmd/client \
  --vindex_base_url http://localhost:8088/vindex/ \
  --out_log_pub_key=example.com/sumdb/vindex+dc01ec4b+AZGQkIvW6u5vtO1DH9NtQUYSUW77q8temEuuXSOlg98+ \
  --lookup=github.com/transparency-dev/tessera
```

### Troubleshooting

Running the Verifiable Index:
- `Failed to update Verifiable Index: failed to get latest checkpoint from DB: no data found`: the clone tool hasn't finished its initial download yet

Querying the Verifiable Index:
- `run failed: lookup for "github.com/transparency-dev/tessera" failed: got non-200 status code: 500`: the Verifiable Index is still building
