# Distributor

The distributor is a RESTful service that makes witnessed checkpoints available.

## Running in Docker

The distributor can be started using `docker compose`.
The following command will bring up the distributor on port `8080`:
```bash
docker compose up -d
```

If using a Raspberry Pi, the above command will fail because no suitable MariaDB image can be
installed. Instead, use this command to install an image that works:
```bash
docker compose -f docker-compose.yaml -f docker-compose.rpi.yaml up -d
```
