# northtalk-go-example

A guestbook served over HTTP by a [NorthTalk](https://northtalk.org) Script,
embedded in Go through [`northtalk.org/go`](https://pkg.go.dev/northtalk.org/go).
Use it as a starting point for your own Host.

- [`guestbook.talk`](guestbook.talk) is the Script: `sign` and `entries`.
- [`guestbook.go`](guestbook.go) is the Host. It grants the Script two
  Capabilities, `db` and `words`. `db` is the `sqlite` Standard Capability on a
  database opened with `northtalk.org/go/sqlite`. `words` is a Go function
  that trims a message and refuses blocked words. A `driver.Pool` pumps the
  Script's Group, so each handler only sends a Request and waits.
- [`guestbook_test.go`](guestbook_test.go) runs the server end to end.

## Run it

Go 1.26 or later; no cgo.

```sh
go run . -addr localhost:8080 -db guestbook.sqlite
curl -X POST localhost:8080/entries -d '{"author": "ada", "message": "hello"}'
curl localhost:8080/entries
```

`POST /entries` answers `201` with the new entry's id, or `422` with the
Script error's code and fields, such as `{"code":"blocked word","word":"spam"}`.

```sh
go test ./...
```

The first run compiles the embedded Zig reactor, which takes a few seconds;
set `NORTHTALK_ZIG_CACHE_DIR` to keep the compiled code between runs.
