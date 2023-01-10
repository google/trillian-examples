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
Content-Length: 302
Content-Type: text/plain
Date: Mon, 09 Jan 2023 19:50:09 GMT

go.sum database tree
15067302
gJyioq6dwR9n+NjEWORfYmH+ubeYm3kH42BBQ0bTpYI=

— sum.golang.org Az3grp1rvWOs5NKlr/xR4GLN/CDyzzakmt1o61uSxYKpK1SerqOP2ClbX/peKUZjYnyomH5Xz7GZG3TpB7OeKn4E2ww=
— test.example.com qTGhC/xIPQ0AXKHK6Y+9ypRfuvMZWPjRXM8NiOg5M1WREn07VQXyuIXM3NRNav3ieVFLUqDRK77PWDmL6w8w84UqeAw=
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
