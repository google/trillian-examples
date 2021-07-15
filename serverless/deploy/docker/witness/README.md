Witness
-------

This [docker-compose.yaml](docker-compose.yaml) file can be used to spin up a witness daemon.

You should set the following environmemt variables (either `export` or with a `.env` file):
name | description
------------------
WITNESS_CONFIG_DIR | Absolute path to a directory containing a witness config file (default: `/etc/witness/config`)
WITNESS_CONFIG_FILE | The name of the witness config file within ${WITNESS_CONFIG_DIR} (default: `witness.config`)
WITNESS_PRIVATE_KEY | The witness private key in note format (*not* the path to the key)


The witness can be started with the following command:

```bash
$ docker-compose -f serverless/deploy/docker/witness/docker-compose.yaml up
Starting witness_witness_1 ... done
Attaching to witness_witness_1
witness_1  | I0714 18:20:19.300495       1 witness.go:88] Connecting to local DB at "/data/witness.sqlite"
witness_1  | I0714 18:20:19.301276       1 witness.go:108] Starting witness server...
```