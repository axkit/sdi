# sdi [![Go Reference](https://pkg.go.dev/badge/github.com/axkit/sdi.svg)](https://pkg.go.dev/github.com/axkit/sdi) [![Build](https://github.com/axkit/sdi/actions/workflows/go.yml/badge.svg)](https://github.com/axkit/sdi/actions/workflows/go.yml) [![codecov](https://codecov.io/gh/axkit/sdi/branch/main/graph/badge.svg)](https://codecov.io/gh/axkit/sdi) [![Go Report Card](https://goreportcard.com/badge/github.com/axkit/sdi)](https://goreportcard.com/report/github.com/axkit/sdi)

Simple Dependency Injection for Go.

`sdi` wires your application objects together using reflection — no code generation, no struct tags required in the common case. Register your objects, call `BuildDependencies()`, and the container automatically assigns interface-typed and pointer-typed fields from the pool of registered objects.

## Installation

```bash
go get github.com/axkit/sdi
```

## Quick start

```go
package main

import (
    "context"
    "github.com/axkit/sdi"
)

type DBer interface{ Query(string) }

type DB struct{}
func (d *DB) Query(q string) { /* ... */ }
func (d *DB) Init(_ context.Context) error  { return nil }
func (d *DB) Start(_ context.Context) error { return nil }

type UserService struct {
    DB DBer // wired automatically
}
func (s *UserService) Init(_ context.Context) error  { return nil }
func (s *UserService) Start(_ context.Context) error { return nil }

func main() {
    cs := sdi.New()
    cs.Add(&DB{})
    cs.Add(&UserService{})
    cs.BuildDependencies()
    cs.InitRequired(context.Background())
    cs.StartRunners(context.Background())
}
```

## Core concepts

### Lifecycle interfaces

| Interface | Method | Called by |
|-----------|--------|-----------|
| `Initializer` | `Init(context.Context) error` | `InitRequired` |
| `Runner` | `Start(context.Context) error` | `StartRunners` |
| `ContaineredService` | both above | both |

Both `InitRequired` and `StartRunners` iterate objects in the container's configured [order](#initialization-order) and stop on the first error.

### Registration methods

| Method | Requires lifecycle? | Participates in unnamed wiring | Participates in lifecycle |
|--------|--------------------|---------------------------------|--------------------------|
| `Add` | yes — at least one of `Initializer` or `Runner` | ✓ | ✓ |
| `Register` | no | ✓ | ✗ |
| `RegisterNamed` | no | only via `sdi:"inject=name"` tag | ✗ |

```go
// object with full lifecycle
cs.Add(&MyService{})

// plain struct for injection only (e.g. config, db connection)
cs.Register(&Config{DSN: "postgres://..."})

// two instances of the same type, distinguished by name
cs.RegisterNamed("readDB",  readConn)
cs.RegisterNamed("writeDB", writeConn)
```

### BuildDependencies

`BuildDependencies()` must be called once after all objects are registered. It scans each object's interface-typed and pointer-typed fields and assigns a matching object from the container. When multiple registered objects satisfy the same field, **the last registered one is used**.

Pre-assigned (non-nil) fields are never overwritten.

## Wiring modes

The mode is set at construction time:

```go
cs := sdi.New()              // Implicit (default)
cs := sdi.New(sdi.Explicit()) // Explicit
```

### Implicit (default)

All exported, nil interface-typed and pointer-typed fields are wired automatically. Unexported fields are ignored unless tagged.

```go
type Service struct {
    Repo   RepoI   // wired automatically (interface)
    Cache  CacheI  // wired automatically (interface)
    Config *Config // wired automatically (concrete pointer)
}
```

### Explicit

Only fields tagged with `sdi:"inject"` or `sdi:"inject=name"` are wired, whether exported or not. Gives full control over every injection point.

```go
type Service struct {
    Repo  RepoI  // NOT wired — no tag
    Cache CacheI `sdi:"inject"` // wired
}
```

### Struct tag reference

| Tag | Applies to | Effect |
|-----|-----------|--------|
| _(none)_ | exported field | wired in Implicit, skipped in Explicit |
| `sdi:"inject"` | exported or unexported field | always wired |
| `sdi:"inject=name"` | exported or unexported field | wired from named registry |
| `sdi:"-"` | exported field | always skipped |

## Initialization order

```go
cs := sdi.New()                // registration order (default)
cs := sdi.New(sdi.Topological()) // dependency order
```

### Registration order (default)

`Init` and `Start` are called in the order objects were registered. **Dependencies must be registered before the objects that depend on them.**

```go
cs.Add(&DB{})          // registered first → Init called first
cs.Add(&UserService{}) // depends on DB    → Init called second
```

### Topological order

The container resolves the wiring graph built by `BuildDependencies` and calls `Init`/`Start` so that every dependency is initialized before its dependents. Registration order no longer matters.

```go
cs := sdi.New(sdi.Topological())
cs.Add(&UserService{}) // depends on DB — registered first, but init runs second
cs.Add(&DB{})          //                  registered second, but init runs first
cs.BuildDependencies()
cs.InitRequired(ctx)   // DB.Init → UserService.Init
```

If a dependency cycle is detected, the container falls back to registration order without panicking.

Options can be combined:

```go
cs := sdi.New(sdi.Explicit(), sdi.Topological())
```

## Named injection

When two objects share the same interface (e.g. read and write DB connections), use `RegisterNamed` and the `sdi:"inject=name"` tag to distinguish them:

```go
cs.RegisterNamed("readDB",  readConn)
cs.RegisterNamed("writeDB", writeConn)

type Repo struct {
    Reader DBer `sdi:"inject=readDB"`
    Writer DBer `sdi:"inject=writeDB"`
}
```

Named objects are not called by `InitRequired` or `StartRunners`, and do not participate in unnamed wiring.

## Real-world example

A typical web application: PostgreSQL, a repository, a service layer, and an HTTP server.

```go
package main

import (
    "context"
    "database/sql"
    "fmt"
    "net/http"

    "github.com/axkit/sdi"
)

// ── Interfaces ────────────────────────────────────────────────────────────────

// Database matches *sql.DB — no wrapper needed.
type Database interface {
    ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
    QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type UserStorer interface {
    Create(ctx context.Context, name string) error
    FindByID(ctx context.Context, id int) (string, error)
}

// ── UserRepository ────────────────────────────────────────────────────────────
// Implements UserStorer. DB is wired to *sql.DB.

type UserRepository struct {
    DB Database // wired to *sql.DB
}

func (r *UserRepository) Init(_ context.Context) error  { return nil }
func (r *UserRepository) Start(_ context.Context) error { return nil }

func (r *UserRepository) Create(ctx context.Context, name string) error {
    _, err := r.DB.ExecContext(ctx, "INSERT INTO users(name) VALUES(?)", name)
    return err
}
func (r *UserRepository) FindByID(ctx context.Context, id int) (string, error) {
    var name string
    err := r.DB.QueryRowContext(ctx, "SELECT name FROM users WHERE id = ?", id).Scan(&name)
    return name, err
}

// ── UserService ───────────────────────────────────────────────────────────────
// Implements UserServicer.

type UserService struct {
    Store UserStorer // wired to *UserRepository
}

func (s *UserService) Init(_ context.Context) error  { return nil }
func (s *UserService) Start(_ context.Context) error { return nil }

func (s *UserService) GetUser(ctx context.Context, id int) (string, error) {
    return s.Store.FindByID(ctx, id)
}

// ── HTTPServer ────────────────────────────────────────────────────────────────
// Svc is wired directly to *UserService (no interface needed).

type HTTPServer struct {
    Svc  *UserService // wired directly to *UserService
    addr string
    mux  *http.ServeMux
}

func NewHTTPServer(addr string) *HTTPServer { return &HTTPServer{addr: addr} }

func (s *HTTPServer) Init(_ context.Context) error {
    s.mux = http.NewServeMux()
    s.mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, r *http.Request) {
        name, err := s.Svc.GetUser(r.Context(), 1)
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        fmt.Fprintln(w, name)
    })
    return nil
}

func (s *HTTPServer) Start(_ context.Context) error {
    go http.ListenAndServe(s.addr, s.mux)
    return nil
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
    cs := sdi.New(sdi.Topological()) // init order determined by dependency graph

    db, err := sql.Open("postgres", "postgres://localhost/mydb")
    if err != nil {
        panic(err)
    }

    cs.Register(db)                                            // *sql.DB satisfies Database
    cs.Add(&UserRepository{})
    cs.Add(&UserService{})
    cs.Add(NewHTTPServer(":8080"))

    cs.BuildDependencies()

    ctx := context.Background()
    if err = cs.InitRequired(ctx); err != nil {
        panic(err)
    }
    // Init order: UserRepository → UserService → HTTPServer
    if err = cs.StartRunners(ctx); err != nil {
        panic(err)
    }

    select {} // keep running
}
```

The dependency graph that `Topological()` resolves:

```text
*sql.DB ──→ UserRepository ──→ UserService ──→ HTTPServer
(Database)    (UserStorer)     (*UserService)
```

Each layer's `Init` runs only after all its dependencies have been initialised, regardless of registration order.

---

## License

MIT — see [LICENSE](./LICENSE).
