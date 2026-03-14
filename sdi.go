// Package sdi provides simple dependency injection for Go.
//
// Objects are registered in a [Container] and wired together by
// [Container.BuildDependencies], which uses reflection to match
// interface-typed and pointer-typed fields to registered objects. Lifecycle
// methods ([Initializer.Init] and [Runner.Start]) are called in the order
// determined by the container's configuration.
//
// Wiring mode and initialization order are selected at construction time:
//
//	cs := sdi.New()                                  // Implicit mode, registration order (defaults)
//	cs := sdi.New(sdi.Explicit())                    // Explicit mode: only tagged fields are wired
//	cs := sdi.New(sdi.Topological())                 // Implicit mode, dependency order
//	cs := sdi.New(sdi.Explicit(), sdi.Topological()) // both options combined
//
// Named injection resolves two objects of the same type into distinct fields:
//
//	cs.RegisterNamed("readDB",  readConn)
//	cs.RegisterNamed("writeDB", writeConn)
//
//	type Repo struct {
//	    Reader dbI `sdi:"inject=readDB"`
//	    Writer dbI `sdi:"inject=writeDB"`
//	}
package sdi

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"unsafe"
)

// mode controls which fields are eligible for automatic wiring.
type mode int

const (
	implicitMode mode = iota
	explicitMode
)

// order controls the sequence in which Init and Start are called.
type order int

const (
	registrationOrder order = iota
	topologicalOrder
)

// Option configures a [Container] at construction time.
type Option func(*Container)

// Implicit returns an Option that wires all exported nil interface-typed and
// pointer-typed fields automatically. This is the default.
func Implicit() Option {
	return func(c *Container) { c.mode = implicitMode }
}

// Explicit returns an Option that wires only fields tagged with
// `sdi:"inject"` or `sdi:"inject=name"`, whether exported or not.
// This applies to both interface-typed and pointer-typed fields.
func Explicit() Option {
	return func(c *Container) { c.mode = explicitMode }
}

// WithLogger returns an Option that enables debug logging via l.
// During [Container.BuildDependencies] each wired field is logged at
// DEBUG level, including a "candidates" attribute when multiple registered
// objects matched and the last one was used. During
// [Container.InitRequired] and [Container.StartRunners] each call
// is logged in execution order.
func WithLogger(l *slog.Logger) Option {
	return func(c *Container) { c.logger = l }
}

// Topological returns an Option that calls [Initializer.Init] and
// [Runner.Start] in dependency order: each object is initialized after all
// objects it depends on. If a dependency cycle is detected, registration
// order is used as a fallback.
func Topological() Option {
	return func(c *Container) { c.order = topologicalOrder }
}

// Initializer is the interface that wraps the Init method.
//
// Init is called by [Container.InitRequired] for each contained object
// that implements Initializer, once, synchronously, and in the container's
// configured order. A cancellable context is passed as an argument.
type Initializer interface {
	Init(context.Context) error
}

// Runner is the interface that wraps the Start method.
//
// Start is called by [Container.StartRunners] for each contained object
// that implements Runner, once, synchronously, and in the container's
// configured order.
//
// If Start has blocking operations (e.g. http.ListenAndServe) it should be
// executed in a separate goroutine. The context should be used for graceful
// shutdown.
type Runner interface {
	Start(context.Context) error
}

// ContaineredService combines [Initializer] and [Runner].
//
// Most services have both an initialisation and a serving phase. Using
// ContaineredService provides compile-time validation of both.
type ContaineredService interface {
	Initializer
	Runner
}

// Container holds registered objects and wires them together.
// Create one with [New].
type Container struct {
	objects    []any
	named      map[string]any
	mode       mode
	order      order
	dependents [][]int      // dependents[i] = indices of objects that depend on object i
	logger     *slog.Logger // non-nil when debug logging is enabled
}

// New returns a new container configured by the provided options.
// Without options, the container uses [Implicit] mode and registration order.
func New(opts ...Option) *Container {
	c := &Container{}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Add registers objects that implement at least one of [Initializer] or
// [Runner]. Panics at startup if neither interface is satisfied.
func (c *Container) Add(o ...any) {
	for i := range o {
		_, isInit := o[i].(Initializer)
		_, isRunner := o[i].(Runner)
		if !isInit && !isRunner {
			panic(fmt.Sprintf("sdi: %T does not implement Initializer or Runner", o[i]))
		}
		c.objects = append(c.objects, o[i])
	}
}

// Register registers objects solely for dependency injection. Unlike
// [Container.Add], no lifecycle interface is required.
// Panics if any object is not a pointer.
func (c *Container) Register(o ...any) {
	for i := range o {
		if reflect.ValueOf(o[i]).Kind() != reflect.Ptr {
			panic(fmt.Sprintf("sdi: %T must be a pointer", o[i]))
		}
		c.objects = append(c.objects, o[i])
	}
}

// RegisterNamed registers an object under a name for explicit named injection.
// Fields tagged with `sdi:"inject=name"` will receive this object during
// [Container.BuildDependencies]. Named objects are not called during
// [Container.InitRequired] or [Container.StartRunners], and do not
// participate in unnamed wiring.
// Panics if name is empty or o is not a pointer.
func (c *Container) RegisterNamed(name string, o any) {
	if name == "" {
		panic("sdi: RegisterNamed requires a non-empty name")
	}
	if reflect.ValueOf(o).Kind() != reflect.Ptr {
		panic(fmt.Sprintf("sdi: %T must be a pointer", o))
	}
	if c.named == nil {
		c.named = make(map[string]any)
	}
	c.named[name] = o
}

// BuildDependencies wires contained objects together. Must be called once
// after all objects have been registered.
//
// For each contained object, it scans interface-typed and pointer-typed fields
// and assigns the last registered object whose type satisfies or matches the
// field's type. When [Topological] order is configured, dependency edges are
// recorded here and used by [Container.InitRequired] and
// [Container.StartRunners].
func (c *Container) BuildDependencies() {
	if c.order == topologicalOrder {
		c.dependents = make([][]int, len(c.objects))
	}
	for i := range c.objects {
		c.setReferenceTo(i, c.objects[i])
	}
}

// InitRequired calls Init on every contained object that implements
// [Initializer], in the container's configured order. Returns the first error
// encountered.
func (c *Container) InitRequired(ctx context.Context) error {
	for _, i := range c.iterOrder() {
		s, ok := c.objects[i].(Initializer)
		if !ok {
			continue
		}
		if c.logger != nil {
			c.logger.Debug("init", "type", fmt.Sprintf("%T", c.objects[i]))
		}
		if err := s.Init(ctx); err != nil {
			return err
		}
	}
	return nil
}

// StartRunners calls Start on every contained object that implements [Runner],
// in the container's configured order. Returns the first error encountered.
func (c *Container) StartRunners(ctx context.Context) error {
	for _, i := range c.iterOrder() {
		s, ok := c.objects[i].(Runner)
		if !ok {
			continue
		}
		if c.logger != nil {
			c.logger.Debug("start", "type", fmt.Sprintf("%T", c.objects[i]))
		}
		if err := s.Start(ctx); err != nil {
			return err
		}
	}
	return nil
}

// iterOrder returns the indices of c.objects in the order that Init and Start
// should be called. For registration order this is simply [0, 1, 2, ...].
// For topological order a Kahn's-algorithm sort is performed; if a cycle is
// detected the result falls back to registration order.
func (c *Container) iterOrder() []int {
	if c.order == topologicalOrder {
		return c.topoSort()
	}
	idx := make([]int, len(c.objects))
	for i := range idx {
		idx[i] = i
	}
	return idx
}

// topoSort returns object indices sorted so that each object appears after all
// objects it depends on (Kahn's algorithm). Falls back to registration order
// if a dependency cycle is detected.
func (c *Container) topoSort() []int {
	n := len(c.objects)
	inDegree := make([]int, n)
	for i := range c.dependents {
		for _, dep := range c.dependents[i] {
			inDegree[dep]++
		}
	}

	queue := make([]int, 0, n)
	for i := 0; i < n; i++ {
		if inDegree[i] == 0 {
			queue = append(queue, i)
		}
	}

	result := make([]int, 0, n)
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		result = append(result, node)
		for _, dep := range c.dependents[node] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(result) != n {
		// Cycle detected — fall back to registration order.
		result = make([]int, n)
		for i := range result {
			result[i] = i
		}
	}
	return result
}

// parseTag splits an sdi struct tag value into its kind and optional name:
//
//	""           → ("", "")
//	"-"          → ("-", "")
//	"inject"     → ("inject", "")
//	"inject=foo" → ("inject", "foo")
func parseTag(tag string) (kind, name string) {
	kind, name, _ = strings.Cut(tag, "=")
	return
}

// setReferenceTo wires all eligible fields of ref by scanning its struct fields.
// Non-struct registered objects (e.g. a named type based on int) are passed
// directly to set, which handles them as a single injectable value.
func (c *Container) setReferenceTo(pos int, ref any) {
	s := reflect.ValueOf(ref)
	t := s.Elem().Type()

	if t.Kind() != reflect.Struct {
		c.set(pos, s, t, "", "")
		return
	}

	for f := 0; f < t.NumField(); f++ {
		sf := t.Field(f)
		fs := s.Elem().Field(f)
		ft := fs.Type()

		if ft.Kind() != reflect.Interface && ft.Kind() != reflect.Ptr {
			continue
		}

		kind, name := parseTag(sf.Tag.Get("sdi"))

		if !fs.CanSet() {
			// Unexported field: only wire if explicitly tagged in any mode.
			if kind != "inject" {
				continue
			}
			// Bypass Go's visibility restriction to obtain a settable Value.
			// UnsafeAddr is safe here: s.Elem() is addressable (derived from a pointer).
			fs = reflect.NewAt(ft, unsafe.Pointer(s.Elem().Field(f).UnsafeAddr())).Elem()
		} else {
			// Exported field.
			if kind == "-" {
				continue
			}
			if c.mode == explicitMode && kind != "inject" {
				continue
			}
		}

		if !fs.IsNil() {
			// Respect values assigned before BuildDependencies was called.
			continue
		}

		if name != "" {
			c.setNamed(fs, ft, name, fmt.Sprintf("%T", ref), sf.Name)
		} else {
			c.set(pos, fs, ft, fmt.Sprintf("%T", ref), sf.Name)
		}
	}
}

// setNamed injects the named object into fs if it is assignable to ft.
// owner and field are used only for debug output.
func (c *Container) setNamed(fs reflect.Value, ft reflect.Type, name, owner, field string) {
	obj, ok := c.named[name]
	if !ok {
		return
	}
	if !reflect.TypeOf(obj).AssignableTo(ft) {
		return
	}
	fs.Set(reflect.NewAt(reflect.TypeOf(obj).Elem(), unsafe.Pointer(reflect.ValueOf(obj).Pointer())))
	if c.logger != nil {
		c.logger.Debug("wire", "field", owner+"."+field, "type", fmt.Sprintf("%T", obj), "name", name)
	}
}

// set assigns the last registered object assignable to ft into fs, skipping
// the object at pos to prevent self-injection. When topological order is
// configured, the dependency edge is recorded in c.dependents.
// owner and field are used only for debug output.
func (c *Container) set(pos int, fs reflect.Value, ft reflect.Type, owner, field string) {
	count, last := 0, -1
	for i := range c.objects {
		if i != pos && reflect.TypeOf(c.objects[i]).AssignableTo(ft) {
			last = i
			count++
		}
	}
	if last == -1 {
		return
	}
	fs.Set(reflect.NewAt(reflect.TypeOf(c.objects[last]).Elem(), unsafe.Pointer(reflect.ValueOf(c.objects[last]).Pointer())))
	if c.logger != nil {
		attrs := []any{"field", owner + "." + field, "type", fmt.Sprintf("%T", c.objects[last])}
		if count > 1 {
			attrs = append(attrs, "candidates", count)
		}
		c.logger.Debug("wire", attrs...)
	}
	if c.dependents != nil {
		// pos depends on last: last must be initialized before pos.
		c.dependents[last] = append(c.dependents[last], pos)
	}
}
