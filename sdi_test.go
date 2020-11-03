package sdi_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/axkit/sdi"
)

func ExampleMain() {
	fmt.Println("hello!")

}

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

type BI interface {
	Name() string
}
type B struct {
	name     string
	AService AI
	CService CI
	Run      *D
	private  struct {
		ES EI
	}
}

func (b *B) Name() string {
	b.Run.Run()

	fmt.Println("here", b.private.ES.String())
	return b.name + fmt.Sprintf(": age=%d, gender=%s", b.AService.Age(), b.CService.Gender())
}

func (b *B) Init(ctx context.Context) error {
	return nil
}

func (b *B) Start(ctx context.Context) error {
	return nil
}

func (b *B) Private() interface{} {
	return &b.private
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

type D struct {
}

func (d *D) Run() {
	fmt.Println("run")
}

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
	//d := D{}
	e := E{25}
	var g G

	cs.Add(&a)
	cs.Add(&b)
	cs.Add(&c)
	//	cs.Add(&d)
	cs.Add(&e)
	cs.Add(&g)
	cs.BuildDependencies()
	if err := cs.InitRequired(context.Background()); err != nil {
		t.Error(err)
	}

	if err := cs.StartRunners(context.Background()); err != nil {
		t.Error(err)
	}

	fmt.Println(b.Name())

	e.v = 99
	a.age = 21

	//c.Set("M")

	fmt.Println(b.Name(), "g=", g)
}
