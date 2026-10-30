// Command northtalk-go-example is a guestbook served over HTTP by a NorthTalk
// Script, embedded in Go through northtalk.org/go. Entries live in SQLite
// through northtalk.org/go/sqlite.
package main

import (
	"flag"
	"log"
	"net/http"
)

func main() {
	addr := flag.String("addr", "localhost:8080", "listen address")
	path := flag.String("db", "guestbook.sqlite", "SQLite database file")
	flag.Parse()
	g, err := Open(*path, []string{"spam"})
	if err != nil {
		log.Fatal(err)
	}
	defer g.Close()
	log.Printf("listening on http://%s/entries", *addr)
	log.Fatal(http.ListenAndServe(*addr, g))
}
