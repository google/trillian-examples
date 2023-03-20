# Addressable witness

## Running a test instance

```
go run . -logtostderr
```

## Testing with HTTPie

```
$ http GET http://localhost:8000/witness/v0/logs
HTTP/1.1 200 OK
Content-Length: 24
Content-Type: application/json
Date: Mon, 09 Jan 2023 19:51:28 GMT

[
    "go.sum database tree"
]
```

```
$ http PUT http://localhost:8000/witness/v0/update Checkpoint=@<(http https://sum.golang.org/latest | base64)
HTTP/1.1 200 OK
Content-Length: 428
Content-Type: text/plain
Date: Tue, 10 Jan 2023 12:07:19 GMT

go.sum database tree
15081221
BoMMe1wd25Tco0mYGROkyqxHZWx+hcSCtMV315Lu6Lg=

— sum.golang.org Az3grilD7vQqTrv66jRgKM84Y80KTprrSt4bKmiOPxgmS5ZVBTeCl6dUvjYnN13R+C8j0jGY1WnUADGMkok3U0m2Zgo=
— test.example.com qTGhC3917mAr6hGh+T55tNjxaej/D/GQivJZRIj7XGdAcQOTqfMWIPgp/v/904LEduN/h2sSRtHUJXUL4b9Gm7Bz4Qc=
— test.example.com /Pa7Y/dUvWMAAAAAQVwkQVi81ArPV7BIaQYrMIxyzY0m8a4BdeeBhSKd3PviPK4/3Sd+NKSc5N7PDTCfAkUSVniN+R2oKIggA4iiDQ==
```

```
$ http GET "http://localhost:8000/witness/v0/logs/go.sum database tree/size"
HTTP/1.1 200 OK
Content-Length: 9
Content-Type: text/plain
Date: Mon, 09 Jan 2023 19:52:05 GMT

15067302
```

## Testing with a bastion

See https://github.com/FiloSottile/mostly-harmless/tree/main/bastion for a
bastion implementation. An example instance runs at `example-bastion.fly.dev:443`
and accepts the example key in this directory.

Note that the example key is public, and only one witness can connect with
it to the example bastion instance at a time.

```
go run . -logtostderr -bastion example-bastion.fly.dev:443
```

```
$ http PUT https://example-bastion.fly.dev/8c126f06698770021621ad797be0100cc54024b9aa1325ae67e6ea2c1fc78fbb/witness/v0/update Checkpoint=@<(http https://sum.golang.org/latest | base64)
```

```
$ http GET https://example-bastion.fly.dev/8c126f06698770021621ad797be0100cc54024b9aa1325ae67e6ea2c1fc78fbb/witness/v0/logs/go.sum%20database%20tree/size
```
