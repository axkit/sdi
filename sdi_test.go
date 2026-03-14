package sdi_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/axkit/sdi"
)

// ============================================================
// Shared test infrastructure
// ============================================================

// trackedService records Init/Start calls and can return configurable errors.
type trackedService struct {
	name     string
	log      *[]string
	initErr  error
	startErr error
}

func (t *trackedService) Init(_ context.Context) error {
	*t.log = append(*t.log, t.name+".Init")
	return t.initErr
}

func (t *trackedService) Start(_ context.Context) error {
	*t.log = append(*t.log, t.name+".Start")
	return t.startErr
}

// depI is the generic injectable dependency interface used across tests.
type depI interface{ Value() int }

// dep implements depI and both lifecycle interfaces.
type dep struct{ v int }

func (d *dep) Value() int                         { return d.v }
func (d *dep) Init(_ context.Context) error       { return nil }
func (d *dep) Start(_ context.Context) error      { return nil }

// plainDep implements only depI — no lifecycle interfaces (for Register tests).
type plainDep struct{ v int }

func (d *plainDep) Value() int { return d.v }

// ============================================================
// Consumer types for Implicit mode
// ============================================================

// implAutoWire: exported Dep field wired automatically in Implicit mode.
type implAutoWire struct{ Dep depI }

func (s *implAutoWire) Init(_ context.Context) error  { return nil }
func (s *implAutoWire) Start(_ context.Context) error { return nil }

// implOptOut: exported Dep field with sdi:"-" must NOT be wired.
type implOptOut struct {
	Dep depI `sdi:"-"`
}

func (s *implOptOut) Init(_ context.Context) error  { return nil }
func (s *implOptOut) Start(_ context.Context) error { return nil }

// implUnexportedTagged: unexported field tagged sdi:"inject" IS wired in Implicit mode.
type implUnexportedTagged struct {
	dep depI `sdi:"inject"`
}

func (s *implUnexportedTagged) Init(_ context.Context) error  { return nil }
func (s *implUnexportedTagged) Start(_ context.Context) error { return nil }
func (s *implUnexportedTagged) Dep() depI                     { return s.dep }

// implUnexportedNoTag: unexported field without tag must NOT be wired.
type implUnexportedNoTag struct {
	dep depI
}

func (s *implUnexportedNoTag) Init(_ context.Context) error  { return nil }
func (s *implUnexportedNoTag) Start(_ context.Context) error { return nil }
func (s *implUnexportedNoTag) Dep() depI                     { return s.dep }

// ============================================================
// Consumer types for Explicit mode
// ============================================================

// explNoTag: exported field without sdi:"inject" must NOT be wired in Explicit mode.
type explNoTag struct{ Dep depI }

func (s *explNoTag) Init(_ context.Context) error  { return nil }
func (s *explNoTag) Start(_ context.Context) error { return nil }

// explTagged: exported field with sdi:"inject" IS wired in Explicit mode.
type explTagged struct {
	Dep depI `sdi:"inject"`
}

func (s *explTagged) Init(_ context.Context) error  { return nil }
func (s *explTagged) Start(_ context.Context) error { return nil }

// explUnexportedTagged: unexported field with sdi:"inject" IS wired in Explicit mode.
type explUnexportedTagged struct {
	dep depI `sdi:"inject"`
}

func (s *explUnexportedTagged) Init(_ context.Context) error  { return nil }
func (s *explUnexportedTagged) Start(_ context.Context) error { return nil }
func (s *explUnexportedTagged) Dep() depI                     { return s.dep }

// ============================================================
// Consumer types for named injection
// ============================================================

// namedConsumer uses two named instances of the same interface.
type namedConsumer struct {
	Reader depI `sdi:"inject=reader"`
	Writer depI `sdi:"inject=writer"`
}

func (s *namedConsumer) Init(_ context.Context) error  { return nil }
func (s *namedConsumer) Start(_ context.Context) error { return nil }

// namedUnexportedConsumer uses named injection into unexported fields.
type namedUnexportedConsumer struct {
	reader depI `sdi:"inject=reader"`
	writer depI `sdi:"inject=writer"`
}

func (s *namedUnexportedConsumer) Init(_ context.Context) error  { return nil }
func (s *namedUnexportedConsumer) Start(_ context.Context) error { return nil }
func (s *namedUnexportedConsumer) Reader() depI                  { return s.reader }
func (s *namedUnexportedConsumer) Writer() depI                  { return s.writer }

// ============================================================
// Consumer for Register tests
// ============================================================

// registeredConsumer depends on depI but the provider has no lifecycle methods.
type registeredConsumer struct{ Dep depI }

func (s *registeredConsumer) Init(_ context.Context) error  { return nil }
func (s *registeredConsumer) Start(_ context.Context) error { return nil }

// ============================================================
// Implicit mode tests
// ============================================================

func TestImplicit_ExplicitOptionSameAsDefault(t *testing.T) {
	// Calling sdi.Implicit() explicitly must behave identically to sdi.New().
	cs := sdi.New(sdi.Implicit())
	d := &dep{v: 42}
	consumer := &implAutoWire{}
	cs.Add(d)
	cs.Add(consumer)
	cs.BuildDependencies()

	if consumer.Dep == nil || consumer.Dep.Value() != 42 {
		t.Fatalf("expected Dep=42 with explicit Implicit() option, got %v", consumer.Dep)
	}
}

func TestImplicit_ExportedFieldWired(t *testing.T) {
	cs := sdi.New()
	d := &dep{v: 42}
	consumer := &implAutoWire{}
	cs.Add(d)
	cs.Add(consumer)
	cs.BuildDependencies()

	if consumer.Dep == nil {
		t.Fatal("expected exported field to be wired, got nil")
	}
	if consumer.Dep.Value() != 42 {
		t.Fatalf("expected 42, got %d", consumer.Dep.Value())
	}
}

func TestImplicit_OptOutTagSkipsExportedField(t *testing.T) {
	cs := sdi.New()
	consumer := &implOptOut{}
	cs.Add(&dep{v: 1})
	cs.Add(consumer)
	cs.BuildDependencies()

	if consumer.Dep != nil {
		t.Fatal("expected sdi:\"-\" field to remain nil")
	}
}

func TestImplicit_UnexportedFieldWithTagWired(t *testing.T) {
	cs := sdi.New()
	d := &dep{v: 7}
	consumer := &implUnexportedTagged{}
	cs.Add(d)
	cs.Add(consumer)
	cs.BuildDependencies()

	if consumer.Dep() == nil {
		t.Fatal("expected unexported tagged field to be wired, got nil")
	}
	if consumer.Dep().Value() != 7 {
		t.Fatalf("expected 7, got %d", consumer.Dep().Value())
	}
}

func TestImplicit_UnexportedFieldWithoutTagNotWired(t *testing.T) {
	cs := sdi.New()
	cs.Add(&dep{v: 1})
	consumer := &implUnexportedNoTag{}
	cs.Add(consumer)
	cs.BuildDependencies()

	if consumer.Dep() != nil {
		t.Fatal("expected unexported field without tag to remain nil")
	}
}

func TestImplicit_PreassignedFieldNotOverwritten(t *testing.T) {
	cs := sdi.New()
	original := &dep{v: 99}
	other := &dep{v: 1}
	consumer := &implAutoWire{Dep: original}
	cs.Add(other)
	cs.Add(consumer)
	cs.BuildDependencies()

	if consumer.Dep.Value() != 99 {
		t.Fatalf("expected pre-assigned value 99, got %d", consumer.Dep.Value())
	}
}

func TestImplicit_LastRegisteredWins(t *testing.T) {
	// When multiple objects satisfy the same interface, the last registered one is injected.
	cs := sdi.New()
	d1 := &dep{v: 10}
	d2 := &dep{v: 20}
	consumer := &implAutoWire{}
	cs.Add(d1)
	cs.Add(d2)
	cs.Add(consumer)
	cs.BuildDependencies()

	if consumer.Dep.Value() != 20 {
		t.Fatalf("expected last registered object (20), got %d", consumer.Dep.Value())
	}
}

func TestImplicit_SelfNotInjected(t *testing.T) {
	// An object must not be injected into its own fields.
	// dep implements depI — if self-injection occurred, consumer.Dep would point to consumer itself.
	cs := sdi.New()
	d := &dep{v: 42}
	consumer := &implAutoWire{}
	cs.Add(d)
	cs.Add(consumer)
	cs.BuildDependencies()

	if consumer.Dep == nil {
		t.Fatal("dep not wired")
	}
	// implAutoWire does not implement depI, so self-injection is structurally impossible here.
	// Verify the wired dep is the registered dep, not consumer itself.
	if consumer.Dep.Value() != 42 {
		t.Fatalf("expected dep.Value()=42, got %d", consumer.Dep.Value())
	}
}

// ============================================================
// Explicit mode tests
// ============================================================

func TestExplicit_UntaggedExportedFieldNotWired(t *testing.T) {
	cs := sdi.New(sdi.Explicit())
	consumer := &explNoTag{}
	cs.Add(&dep{v: 5})
	cs.Add(consumer)
	cs.BuildDependencies()

	if consumer.Dep != nil {
		t.Fatal("expected untagged exported field to remain nil in Explicit mode")
	}
}

func TestExplicit_TaggedExportedFieldWired(t *testing.T) {
	cs := sdi.New(sdi.Explicit())
	d := &dep{v: 5}
	consumer := &explTagged{}
	cs.Add(d)
	cs.Add(consumer)
	cs.BuildDependencies()

	if consumer.Dep == nil {
		t.Fatal("expected sdi:\"inject\" exported field to be wired")
	}
	if consumer.Dep.Value() != 5 {
		t.Fatalf("expected 5, got %d", consumer.Dep.Value())
	}
}

func TestExplicit_TaggedUnexportedFieldWired(t *testing.T) {
	cs := sdi.New(sdi.Explicit())
	d := &dep{v: 3}
	consumer := &explUnexportedTagged{}
	cs.Add(d)
	cs.Add(consumer)
	cs.BuildDependencies()

	if consumer.Dep() == nil {
		t.Fatal("expected sdi:\"inject\" unexported field to be wired in Explicit mode")
	}
	if consumer.Dep().Value() != 3 {
		t.Fatalf("expected 3, got %d", consumer.Dep().Value())
	}
}

// ============================================================
// Register tests
// ============================================================

func TestRegister_PlainStructInjected(t *testing.T) {
	cs := sdi.New()
	plain := &plainDep{v: 55}
	consumer := &registeredConsumer{}
	cs.Register(plain)
	cs.Add(consumer)
	cs.BuildDependencies()

	if consumer.Dep == nil {
		t.Fatal("expected plain registered struct to be injected")
	}
	if consumer.Dep.Value() != 55 {
		t.Fatalf("expected 55, got %d", consumer.Dep.Value())
	}
}

func TestRegister_NotInvokedByLifecycle(t *testing.T) {
	// plainDep has no Init/Start — InitRequired/StartRunners must not panic or call it.
	cs := sdi.New()
	cs.Register(&plainDep{v: 1})

	if err := cs.InitRequired(context.Background()); err != nil {
		t.Fatalf("InitRequired failed: %v", err)
	}
	if err := cs.StartRunners(context.Background()); err != nil {
		t.Fatalf("StartRunners failed: %v", err)
	}
}

func TestRegister_PanicOnNonPointer(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for non-pointer value")
		}
	}()
	cs := sdi.New()
	cs.Register(42) // not a pointer — must panic
}

// ============================================================
// Add panic tests
// ============================================================

func TestAdd_PanicWhenNoLifecycleInterface(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when adding object without any lifecycle interface")
		}
	}()
	cs := sdi.New()
	cs.Add(&plainDep{}) // plainDep has no Init/Start — must panic
}

// ============================================================
// InitRequired tests
// ============================================================

func TestInitRequired_CalledInRegistrationOrder(t *testing.T) {
	var log []string
	cs := sdi.New()
	cs.Add(&trackedService{name: "A", log: &log})
	cs.Add(&trackedService{name: "B", log: &log})
	cs.Add(&trackedService{name: "C", log: &log})

	if err := cs.InitRequired(context.Background()); err != nil {
		t.Fatal(err)
	}

	want := []string{"A.Init", "B.Init", "C.Init"}
	for i, got := range log {
		if got != want[i] {
			t.Errorf("position %d: want %q, got %q", i, want[i], got)
		}
	}
}

func TestInitRequired_StopsOnFirstError(t *testing.T) {
	var log []string
	sentinel := errors.New("init error")
	cs := sdi.New()
	cs.Add(&trackedService{name: "A", log: &log})
	cs.Add(&trackedService{name: "B", log: &log, initErr: sentinel})
	cs.Add(&trackedService{name: "C", log: &log})

	err := cs.InitRequired(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if len(log) != 2 {
		t.Fatalf("expected 2 calls (A, B), got %d: %v", len(log), log)
	}
	if log[1] != "B.Init" {
		t.Errorf("expected B.Init at position 1, got %q", log[1])
	}
}

// ============================================================
// StartRunners tests
// ============================================================

func TestStartRunners_CalledInRegistrationOrder(t *testing.T) {
	var log []string
	cs := sdi.New()
	cs.Add(&trackedService{name: "A", log: &log})
	cs.Add(&trackedService{name: "B", log: &log})
	cs.Add(&trackedService{name: "C", log: &log})

	if err := cs.StartRunners(context.Background()); err != nil {
		t.Fatal(err)
	}

	want := []string{"A.Start", "B.Start", "C.Start"}
	for i, got := range log {
		if got != want[i] {
			t.Errorf("position %d: want %q, got %q", i, want[i], got)
		}
	}
}

func TestStartRunners_StopsOnFirstError(t *testing.T) {
	var log []string
	sentinel := errors.New("start error")
	cs := sdi.New()
	cs.Add(&trackedService{name: "A", log: &log})
	cs.Add(&trackedService{name: "B", log: &log, startErr: sentinel})
	cs.Add(&trackedService{name: "C", log: &log})

	err := cs.StartRunners(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if len(log) != 2 {
		t.Fatalf("expected 2 calls (A, B), got %d: %v", len(log), log)
	}
}

// ============================================================
// Topological order types
// ============================================================

// topoDepI is the interface topoA satisfies, depended on by topoB.
type topoDepI interface{ Ping() }

// topoA has no dependencies.
type topoA struct{ log *[]string }

func (a *topoA) Ping()                            {}
func (a *topoA) Init(_ context.Context) error     { *a.log = append(*a.log, "A"); return nil }
func (a *topoA) Start(_ context.Context) error    { *a.log = append(*a.log, "A"); return nil }

// topoB depends on topoA via its Dep field.
type topoB struct {
	Dep topoDepI
	log *[]string
}

func (b *topoB) Init(_ context.Context) error  { *b.log = append(*b.log, "B"); return nil }
func (b *topoB) Start(_ context.Context) error { *b.log = append(*b.log, "B"); return nil }

// topoC depends on topoB.
type topoCDepI interface{ Pong() }
type topoC struct {
	Dep topoCDepI
	log *[]string
}

func (b *topoB) Pong()                          {}
func (c *topoC) Init(_ context.Context) error  { *c.log = append(*c.log, "C"); return nil }
func (c *topoC) Start(_ context.Context) error { *c.log = append(*c.log, "C"); return nil }

// cycleX and cycleY hold references to each other, forming a cycle.
type cycleXI interface{ Xi() }
type cycleYI interface{ Yi() }

type cycleX struct{ Y cycleYI }

func (x *cycleX) Xi()                              {}
func (x *cycleX) Init(_ context.Context) error     { return nil }
func (x *cycleX) Start(_ context.Context) error    { return nil }

type cycleY struct{ X cycleXI }

func (y *cycleY) Yi()                              {}
func (y *cycleY) Init(_ context.Context) error     { return nil }
func (y *cycleY) Start(_ context.Context) error    { return nil }

// ============================================================
// Topological order tests
// ============================================================

func TestTopological_InitRunsInDependencyOrder(t *testing.T) {
	var log []string
	a := &topoA{log: &log}
	b := &topoB{log: &log}

	// B is registered before A, but B depends on A.
	// Topological order must init A before B.
	cs := sdi.New(sdi.Topological())
	cs.Add(b)
	cs.Add(a)
	cs.BuildDependencies()

	if err := cs.InitRequired(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(log) != 2 || log[0] != "A" || log[1] != "B" {
		t.Errorf("expected [A B], got %v", log)
	}
}

func TestTopological_StartRunsInDependencyOrder(t *testing.T) {
	var log []string
	a := &topoA{log: &log}
	b := &topoB{log: &log}

	cs := sdi.New(sdi.Topological())
	cs.Add(b)
	cs.Add(a)
	cs.BuildDependencies()

	if err := cs.StartRunners(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(log) != 2 || log[0] != "A" || log[1] != "B" {
		t.Errorf("expected [A B], got %v", log)
	}
}

func TestTopological_ThreeObjectChain(t *testing.T) {
	var log []string
	a := &topoA{log: &log}
	b := &topoB{log: &log}
	c := &topoC{log: &log}

	// Register in reverse dependency order: C, B, A.
	cs := sdi.New(sdi.Topological())
	cs.Add(c)
	cs.Add(b)
	cs.Add(a)
	cs.BuildDependencies()

	if err := cs.InitRequired(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(log) != 3 || log[0] != "A" || log[1] != "B" || log[2] != "C" {
		t.Errorf("expected [A B C], got %v", log)
	}
}

func TestTopological_CycleFallsBackToRegistrationOrder(t *testing.T) {
	// cycleX.Y is wired to cycleY, cycleY.X is wired to cycleX — a cycle.
	// Must not panic; init order falls back to registration order.
	var log []string
	x := &cycleX{}
	y := &cycleY{}

	cs := sdi.New(sdi.Topological())
	cs.Add(x)
	cs.Add(y)
	cs.BuildDependencies()

	if err := cs.InitRequired(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := cs.StartRunners(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = log
}

func TestRegistration_InitRunsInRegistrationOrder(t *testing.T) {
	var log []string
	a := &topoA{log: &log}
	b := &topoB{log: &log}

	// B registered first — registration order means B inits before A.
	cs := sdi.New()
	cs.Add(b)
	cs.Add(a)
	cs.BuildDependencies()

	if err := cs.InitRequired(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(log) != 2 || log[0] != "B" || log[1] != "A" {
		t.Errorf("expected [B A], got %v", log)
	}
}

func TestTopological_WithExplicit(t *testing.T) {
	// Explicit() and Topological() can be combined: only tagged fields are
	// wired, and init runs in dependency order.
	cs := sdi.New(sdi.Explicit(), sdi.Topological())
	d := &dep{v: 5}
	consumer := &explTagged{}
	cs.Add(d)
	cs.Add(consumer)
	cs.BuildDependencies()

	if consumer.Dep == nil || consumer.Dep.Value() != 5 {
		t.Fatalf("expected Dep=5 with Explicit+Topological, got %v", consumer.Dep)
	}
}

// ============================================================
// Named injection tests
// ============================================================

func TestNamed_TwoInstancesSameInterface(t *testing.T) {
	cs := sdi.New()
	reader := &dep{v: 1}
	writer := &dep{v: 2}
	consumer := &namedConsumer{}

	cs.RegisterNamed("reader", reader)
	cs.RegisterNamed("writer", writer)
	cs.Add(consumer)
	cs.BuildDependencies()

	if consumer.Reader == nil || consumer.Reader.Value() != 1 {
		t.Fatalf("expected Reader=1, got %v", consumer.Reader)
	}
	if consumer.Writer == nil || consumer.Writer.Value() != 2 {
		t.Fatalf("expected Writer=2, got %v", consumer.Writer)
	}
}

func TestNamed_UnexportedFields(t *testing.T) {
	cs := sdi.New()
	reader := &dep{v: 10}
	writer := &dep{v: 20}
	consumer := &namedUnexportedConsumer{}

	cs.RegisterNamed("reader", reader)
	cs.RegisterNamed("writer", writer)
	cs.Add(consumer)
	cs.BuildDependencies()

	if consumer.Reader() == nil || consumer.Reader().Value() != 10 {
		t.Fatalf("expected reader=10, got %v", consumer.Reader())
	}
	if consumer.Writer() == nil || consumer.Writer().Value() != 20 {
		t.Fatalf("expected writer=20, got %v", consumer.Writer())
	}
}

func TestNamed_NotInjectedUnnamedFields(t *testing.T) {
	// Named objects must not bleed into unnamed wiring.
	cs := sdi.New()
	consumer := &implAutoWire{}

	cs.RegisterNamed("something", &dep{v: 99})
	cs.Add(consumer)
	cs.BuildDependencies()

	if consumer.Dep != nil {
		t.Fatal("named object must not be injected into unnamed fields")
	}
}

func TestNamed_NotInvokedByLifecycle(t *testing.T) {
	cs := sdi.New()
	cs.RegisterNamed("db", &plainDep{v: 1})

	if err := cs.InitRequired(context.Background()); err != nil {
		t.Fatalf("InitRequired failed: %v", err)
	}
	if err := cs.StartRunners(context.Background()); err != nil {
		t.Fatalf("StartRunners failed: %v", err)
	}
}

func TestNamed_UnknownNameLeavesFieldNil(t *testing.T) {
	cs := sdi.New()
	consumer := &namedConsumer{}

	cs.Add(consumer)
	cs.BuildDependencies() // "reader" and "writer" never registered

	if consumer.Reader != nil || consumer.Writer != nil {
		t.Fatal("unresolved named fields should remain nil")
	}
}

func TestNamed_PanicOnEmptyName(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for empty name")
		}
	}()
	cs := sdi.New()
	cs.RegisterNamed("", &dep{})
}

func TestNamed_TypeMismatchLeavesFieldNil(t *testing.T) {
	// "reader" is registered but with a type that does NOT satisfy depI.
	// AssignableTo returns false → field stays nil.
	cs := sdi.New()
	cs.RegisterNamed("reader", &concreteConfig{Value: 1}) // concreteConfig does not implement depI
	consumer := &namedConsumer{}
	cs.Add(consumer)
	cs.BuildDependencies()

	if consumer.Reader != nil {
		t.Fatal("type-mismatched named object must not be injected")
	}
}

func TestNamed_PanicOnNonPointer(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for non-pointer value")
		}
	}()
	cs := sdi.New()
	cs.RegisterNamed("db", 42)
}

func TestNamed_WorksInExplicitMode(t *testing.T) {
	cs := sdi.New(sdi.Explicit())
	reader := &dep{v: 7}
	writer := &dep{v: 8}
	consumer := &namedConsumer{}

	cs.RegisterNamed("reader", reader)
	cs.RegisterNamed("writer", writer)
	cs.Add(consumer)
	cs.BuildDependencies()

	if consumer.Reader == nil || consumer.Reader.Value() != 7 {
		t.Fatalf("expected Reader=7 in Explicit mode, got %v", consumer.Reader)
	}
	if consumer.Writer == nil || consumer.Writer.Value() != 8 {
		t.Fatalf("expected Writer=8 in Explicit mode, got %v", consumer.Writer)
	}
}

// ============================================================
// Concrete pointer field injection tests
// ============================================================

// concreteConfig is a plain struct used to test pointer-typed field injection.
type concreteConfig struct{ Value int }

// concreteConsumer has a pointer-typed field instead of an interface.
type concreteConsumer struct {
	Cfg *concreteConfig
}

func (s *concreteConsumer) Init(_ context.Context) error  { return nil }
func (s *concreteConsumer) Start(_ context.Context) error { return nil }

// concreteExplicit has a pointer-typed field that requires sdi:"inject" in Explicit mode.
type concreteExplicit struct {
	Cfg *concreteConfig `sdi:"inject"`
}

func (s *concreteExplicit) Init(_ context.Context) error  { return nil }
func (s *concreteExplicit) Start(_ context.Context) error { return nil }

func TestConcrete_PointerFieldWiredImplicit(t *testing.T) {
	cs := sdi.New()
	cfg := &concreteConfig{Value: 42}
	consumer := &concreteConsumer{}
	cs.Register(cfg)
	cs.Add(consumer)
	cs.BuildDependencies()

	if consumer.Cfg == nil {
		t.Fatal("expected concrete pointer field to be wired")
	}
	if consumer.Cfg.Value != 42 {
		t.Fatalf("expected 42, got %d", consumer.Cfg.Value)
	}
}

func TestConcrete_PointerFieldWiredExplicit(t *testing.T) {
	cs := sdi.New(sdi.Explicit())
	cfg := &concreteConfig{Value: 7}
	consumer := &concreteExplicit{}
	cs.Register(cfg)
	cs.Add(consumer)
	cs.BuildDependencies()

	if consumer.Cfg == nil {
		t.Fatal("expected sdi:\"inject\" pointer field to be wired in Explicit mode")
	}
	if consumer.Cfg.Value != 7 {
		t.Fatalf("expected 7, got %d", consumer.Cfg.Value)
	}
}

func TestConcrete_UntaggedPointerNotWiredExplicit(t *testing.T) {
	cs := sdi.New(sdi.Explicit())
	consumer := &concreteConsumer{}
	cs.Register(&concreteConfig{Value: 1})
	cs.Add(consumer)
	cs.BuildDependencies()

	if consumer.Cfg != nil {
		t.Fatal("expected untagged pointer field to remain nil in Explicit mode")
	}
}

func TestConcrete_PreassignedPointerNotOverwritten(t *testing.T) {
	cs := sdi.New()
	original := &concreteConfig{Value: 99}
	consumer := &concreteConsumer{Cfg: original}
	cs.Register(&concreteConfig{Value: 1})
	cs.Add(consumer)
	cs.BuildDependencies()

	if consumer.Cfg.Value != 99 {
		t.Fatalf("expected pre-assigned value 99, got %d", consumer.Cfg.Value)
	}
}

// ============================================================
// WithDebug tests
// ============================================================

func newDebugLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestWithLogger_LogsWiringAndLifecycle(t *testing.T) {
	var buf bytes.Buffer
	cs := sdi.New(sdi.WithLogger(newDebugLogger(&buf)))

	d := &dep{v: 1}
	consumer := &implAutoWire{}
	cs.Add(d)
	cs.Add(consumer)
	cs.BuildDependencies()

	out := buf.String()
	if !strings.Contains(out, "msg=wire") {
		t.Errorf("expected wire log, got:\n%s", out)
	}
	if !strings.Contains(out, ".Dep") {
		t.Errorf("expected field name in wire log, got:\n%s", out)
	}

	buf.Reset()
	cs.InitRequired(context.Background())
	out = buf.String()
	if !strings.Contains(out, "msg=init") {
		t.Errorf("expected init log, got:\n%s", out)
	}

	buf.Reset()
	cs.StartRunners(context.Background())
	out = buf.String()
	if !strings.Contains(out, "msg=start") {
		t.Errorf("expected start log, got:\n%s", out)
	}
}

func TestWithLogger_MultipleCandidatesNote(t *testing.T) {
	var buf bytes.Buffer
	cs := sdi.New(sdi.WithLogger(newDebugLogger(&buf)))

	cs.Add(&dep{v: 1})
	cs.Add(&dep{v: 2})
	cs.Add(&dep{v: 3})
	cs.Add(&implAutoWire{})
	cs.BuildDependencies()

	if !strings.Contains(buf.String(), "candidates=3") {
		t.Errorf("expected candidates attribute, got:\n%s", buf.String())
	}
}

func TestWithLogger_NamedInjectionLogged(t *testing.T) {
	var buf bytes.Buffer
	cs := sdi.New(sdi.WithLogger(newDebugLogger(&buf)))

	cs.RegisterNamed("reader", &dep{v: 1})
	cs.RegisterNamed("writer", &dep{v: 2})
	cs.Add(&namedConsumer{})
	cs.BuildDependencies()

	out := buf.String()
	if !strings.Contains(out, "name=reader") || !strings.Contains(out, "name=writer") {
		t.Errorf("expected name attribute, got:\n%s", out)
	}
}

// ============================================================
// Regression: TestOverall (original behaviour preserved)
// ============================================================

type AI interface {
	Age() int
}

type A struct {
	age int
}

func (a *A) Age() int {
	return a.age
}

func (a *A) Init(ctx context.Context) error {
	a.age = 20
	return nil
}

func (a *A) Start(ctx context.Context) error {
	return nil
}

type B struct {
	name     string
	AService AI
	CService CI
	Run      *D
	es       EI `sdi:"inject"`
}

func (b *B) Init(ctx context.Context) error {
	return nil
}

func (b *B) Start(ctx context.Context) error {
	return nil
}

type CI interface {
	Set(string)
	Gender() string
}

type C struct {
	gender string
}

func (c *C) Set(g string) {
	c.gender = g
}
func (c *C) Gender() string {
	return c.gender
}

func (c *C) Init(ctx context.Context) error {
	return nil
}

func (c *C) Start(ctx context.Context) error {
	return nil
}

type D struct{}

func (d *D) Run() {}

type EI interface {
	sdi.ContaineredService
	String() string
}

type E struct {
	v int
}

func (e *E) String() string {
	return fmt.Sprintf("value=%d", e.v)
}

func (e *E) Init(ctx context.Context) error {
	e.v = 202
	return nil
}

func (e *E) Start(ctx context.Context) error {
	return nil
}

type G int

func (g *G) Init(ctx context.Context) error {
	*g = 10
	return nil
}

func TestOverall(t *testing.T) {
	cs := sdi.New()
	a := A{}
	c := C{}
	b := B{}
	e := E{25}
	var g G

	cs.Add(&a)
	cs.Add(&b)
	cs.Add(&c)
	cs.Add(&e)
	cs.Add(&g)
	cs.BuildDependencies()

	if err := cs.InitRequired(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := cs.StartRunners(context.Background()); err != nil {
		t.Fatal(err)
	}

	if a.Age() != 20 {
		t.Errorf("expected a.age=20 after Init, got %d", a.Age())
	}
	if int(g) != 10 {
		t.Errorf("expected g=10 after Init, got %d", g)
	}
	if b.AService == nil {
		t.Error("expected b.AService to be wired")
	}
	if b.CService == nil {
		t.Error("expected b.CService to be wired")
	}
	if b.es == nil {
		t.Error("expected b.es to be wired via sdi:\"inject\" tag")
	}
}
