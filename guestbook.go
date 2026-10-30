package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	talk "northtalk.org/go"
	"northtalk.org/go/driver"
	"northtalk.org/go/sqlite"
)

//go:embed guestbook.talk
var source string

const schema = "CREATE TABLE IF NOT EXISTS entries (id INTEGER PRIMARY KEY, author TEXT NOT NULL, message TEXT NOT NULL)"

// Guestbook serves the guestbook Script over HTTP. The Pool pumps its Group,
// so a handler only sends a Request and waits for the result.
type Guestbook struct {
	db     *sqlite.Database
	pool   *driver.Pool
	script *talk.Script
}

// Open opens the database at path, grants the Script `db` and `words`, and
// loads it.
func Open(path string, blocked []string) (*Guestbook, error) {
	db, err := sqlite.Open(path)
	if err != nil {
		return nil, err
	}
	g := &Guestbook{db: db, pool: driver.NewPool(driver.PoolOptions{Workers: 2})}
	if err := g.load(blocked); err != nil {
		g.Close()
		return nil, err
	}
	return g, nil
}

func (g *Guestbook) load(blocked []string) error {
	if err := g.db.Exec(schema); err != nil {
		return err
	}
	core := talk.New()
	costs := talk.Costs{}
	for _, op := range []string{"query", "change", "begin", "commit", "rollback"} {
		costs[op] = talk.Cost{Fuel: 10}
	}
	sql, err := core.SqliteCapability(sqlite.Databases(map[string]*sqlite.Database{"guestbook": g.db}), costs, 1)
	if err != nil {
		return err
	}
	db, err := sql.Grant([]string{"query", "change"}, talk.SqliteBinding{Database: "guestbook", MaxRows: 100})
	if err != nil {
		return err
	}
	words, err := core.DefineCapability("words", checkOperation())
	if err != nil {
		return err
	}
	check, err := words.Grant([]string{"check"}, blocked)
	if err != nil {
		return err
	}
	member := g.pool.NewGroup(core, talk.GroupOptions{Name: "guestbook"})
	return member.Do(func(group *talk.Group) error {
		g.script, err = group.Load(talk.LoadOptions{Name: "guestbook", Source: source, Grants: map[string]*talk.Grant{"db": db, "words": check}})
		return err
	})
}

// checkOperation is the one Capability the Host defines itself: it trims a
// message, and refuses an empty one or one holding a blocked word. The
// Grant's binding is the blocked list.
func checkOperation() talk.Operation {
	return talk.Operation{
		Name: "check", Mode: talk.Immediate, Args: []talk.Shape{talk.TextShape}, Result: talk.TextShape, Cost: talk.Cost{Fuel: 5},
		Errors: []talk.ErrorDecl{{Code: "empty message"}, {Code: "blocked word", Fields: []talk.Field{{Key: "word", Shape: talk.TextShape}}}},
		Do: func(c *talk.Call, args []talk.Value) (talk.Value, error) {
			text, _ := args[0].AsText()
			text = strings.TrimSpace(text)
			if text == "" {
				return talk.Nothing, &talk.ScriptError{Code: "empty message"}
			}
			for _, word := range c.Binding().([]string) {
				if strings.Contains(strings.ToLower(text), word) {
					w, _ := talk.Text(word)
					data, _ := talk.Map(talk.KV("word", w))
					return talk.Nothing, &talk.ScriptError{Code: "blocked word", Data: data}
				}
			}
			return talk.Text(text)
		},
	}
}

// Close stops the Pool, then closes the database.
func (g *Guestbook) Close() error {
	g.pool.Close()
	return g.db.Close()
}

func (g *Guestbook) request(ctx context.Context, name string, args ...string) (talk.Value, error) {
	values := make([]talk.Value, len(args))
	for i, a := range args {
		v, err := talk.Text(a)
		if err != nil {
			return talk.Nothing, err
		}
		values[i] = v
	}
	_, p, err := g.script.Request(ctx, talk.Message{Name: name, Args: values})
	if err != nil {
		return talk.Nothing, err
	}
	select {
	case <-p.Done():
	case <-ctx.Done():
		return talk.Nothing, ctx.Err()
	}
	v, failure := p.Result()
	if failure != nil {
		return talk.Nothing, failure
	}
	return v, nil
}

// ServeHTTP serves GET /entries, which lists the entries, and POST /entries,
// which signs the guestbook with a JSON {"author", "message"} body.
func (g *Guestbook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/entries" {
		http.NotFound(w, r)
		return
	}
	var (
		v      talk.Value
		err    error
		status = http.StatusOK
	)
	switch r.Method {
	case http.MethodGet:
		v, err = g.request(r.Context(), "entries")
	case http.MethodPost:
		var body struct{ Author, Message string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		v, err = g.request(r.Context(), "sign", body.Author, body.Message)
		status = http.StatusCreated
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if failure := (*talk.ScriptError)(nil); errors.As(err, &failure) {
		// A Script error is the client's: the Value carries its code and data.
		v, err = errorValue(failure)
		status = http.StatusUnprocessableEntity
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	b, err := talk.EncodeJSON(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(b)
}

// errorValue is the client's view of a Script error: its code and declared
// fields, without the Script location. A Request whose Run errored fails with
// `send failed`, carrying the Run's error.
func errorValue(failure *talk.ScriptError) (talk.Value, error) {
	inner := failure.Data.Get("error")
	if failure.Code != "send failed" || inner.Kind() != talk.KindMap {
		code, err := talk.Text(failure.Code)
		if err != nil {
			return talk.Nothing, err
		}
		return talk.Map(talk.KV("code", code))
	}
	var pairs []talk.Pair
	for _, p := range inner.Entries() {
		switch p.Key {
		case "message", "capability", "operation", "at":
		default:
			pairs = append(pairs, p)
		}
	}
	return talk.Map(pairs...)
}
