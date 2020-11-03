/* Package sdi provides Simple Dependency Injection functionality.

 */
package sdi

import (
	"context"
	"fmt"
	"reflect"
	"unsafe"
)

// Initializer is the interface that wraps the basic Init method.
//
// Init is invocated inside container's InitRequired() for each contairened object
// implementing Initializer interface, once, sychronously, and in sequential order.
//
// Cancellable Context passed as an argument.
type Initializer interface {
	Init(context.Context) error
}

// Runner is the interface that wraps the basic Start method.
//
// Start is invocated inside container's StartRunners() for each contairened object
// implementing Starter interface, once, sychronously and in sequential order.
//
// If Start has blocking operations (e.g. http.ListenAndServe) it should be
// executed in a separate goroutine.
//
// Cancellable Context passed as an argument and should be used
// for gracefull shutdown.
type Runner interface {
	Start(context.Context) error
}

// ContaineredService is the interface what wraps two interfaces
// Initializer and Runner.
//
// Typical service usually has initialisation and serving parts. Using
// ContaineredService interface provides compile time type validation.
type ContaineredService interface {
	Initializer
	Runner
}

// Container is the interface what wraps functions providing simple
// dependency injection functionality.
//
// AddService adds objects into container implementing interface ContaineredService.
//
// Add adds object into container implementing Initializer, Runner or Globalizer interfaces.
//
// BuildDependencies links objects added into container between each other.
//
// InitRequired call Init for each containerised object implementing Initialized interface.
// Returns error if calling Init returns error and breaks initializing following Initializers.
//
// StartRunners calls Start for each containerised object implementing Runner interface.
// Returns error if calling Start returns error and breaks launching following Runners.
type Container interface {
	AddService(...ContaineredService)
	Add(...interface{})
	BuildDependencies()
	InitRequired(context.Context) error
	StartRunners(context.Context) error
}

type Privater interface {
	Private() interface{}
}

// Globalizer is the interface that wraps the basic Global method.
//
// Implementing interface Globalizer is a simple way of injecting arbitrary entity
// if there are sense of implementing Runner or Initializer interfaces.
type Globalizer interface {
	Global()
}

// Global implements Globalizer interface.
type Global struct {
}

func (g *Global) Global() {}

// SimpleContainer holds references to containered objects
// and implements Container interface.
type SimpleContainer struct {
	objects []interface{}
}

// New returns container for objects.
func New() *SimpleContainer {
	return &SimpleContainer{}
}

var _ Container = &SimpleContainer{}

// AddService add objects implementing interface ContaineredService into container.
func (c *SimpleContainer) AddService(o ...ContaineredService) {
	for i := range o {
		c.objects = append(c.objects, o[i])
	}
}

// Add adds an object into container.
// It panics if parameter:
// - is not a pointer
// - does not implement Initializer, Runner or Globalizer interface.
func (c *SimpleContainer) Add(o ...interface{}) {

	for i := range o {
		_, in := o[i].(Initializer)
		_, ru := o[i].(Runner)
		_, gl := o[i].(Globalizer)
		if !in && !ru && !gl {
			panic(fmt.Sprintf("%T does not implement Runner, Initializer or Globalizer interfaces", o[i]))
		}

		c.objects = append(c.objects, o[i])
	}
}

// BuildDependencies links containered objects. The method should be called
// once after adding all necessary objects into container.
func (c *SimpleContainer) BuildDependencies() {
	c.buildDependencies()
}

// InitRequired inits each containered object if it implements
// Initializer interface.
func (c *SimpleContainer) InitRequired(ctx context.Context) error {
	for i := range c.objects {
		s, ok := c.objects[i].(Initializer)
		if !ok {
			continue
		}
		if err := s.Init(ctx); err != nil {
			return err
		}
	}
	return nil
}

// StartRunners starts runner of each containered object if it
// implements Runner interface.
//
// Starts one in the order they've been added into container.
func (c *SimpleContainer) StartRunners(ctx context.Context) error {
	for i := range c.objects {
		s, ok := c.objects[i].(Runner)
		if !ok {
			continue
		}
		if err := s.Start(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (c *SimpleContainer) buildDependencies() {
	for i := range c.objects {
		c.setReferenceTo(i, c.objects[i])
		if pa, ok := c.objects[i].(Privater); ok {
			obj := pa.Private()
			c.setReferenceTo(i, obj)
		}
	}
}

func (c *SimpleContainer) setReferenceTo(pos int, ref interface{}) {

	s := reflect.ValueOf(ref)
	t := s.Elem().Type()

	if t.Kind() != reflect.Struct {
		c.set(pos, s, t)
		return
	}

	// pass through the struct fields.
	for f := 0; f < t.NumField(); f++ {

		fs := s.Elem().Field(f)
		ft := fs.Type()

		if fs.CanSet() == false {
			continue
		}

		if ft.Kind() != reflect.Interface {
			continue
		}

		if fs.IsNil() == false {
			// if assigned already by user before.
			continue
		}
		c.set(pos, fs, ft)
	}

}

func (c *SimpleContainer) set(pos int, fs reflect.Value, ft reflect.Type) {
	for i := range c.objects {
		if pos == i {
			// pass reference to itself.
			continue
		}

		md := reflect.TypeOf(c.objects[i])

		if !md.AssignableTo(ft) {
			// pass not complaint
			continue
		}
		v := reflect.NewAt(reflect.TypeOf(c.objects[i]).Elem(), unsafe.Pointer(reflect.ValueOf(c.objects[i]).Pointer()))
		fs.Set(v)
	}
}

/*
func (c *SimpleContainer) setReferenceBackup(pos int, ref interface{}) {

	s := reflect.ValueOf(ref)
	t := s.Elem().Type()

	// однозначно верим в том, что c.service[i] является структурой
	// TODO: поставить проверку
	if t.Kind() != reflect.Struct {
		//panic(fmt.Sprintf("%T, %#v", ref, s))
	}

	//fmt.Printf("service[%d] s=%s, t=%s\n", i, s.String(), t.String())

	// для каждого атрибута структуры, перебираем атрибуты.
	for f := 0; f < t.NumField(); f++ {

		fs := s.Elem().Field(f)
		ft := fs.Type()

		if fs.CanSet() == false {
			continue
		}

		if ft.Kind() != reflect.Interface {
			continue
		}

		if fs.IsNil() == false {
			// если присвоено, например конструктором сервиса.
			continue
		}

		for k := range c.objects {
			if pos == k {
				continue
			}
			//fmt.Printf("сервис %d, %T %T %#v\n", k, c.service[k], fs.Elem(), reflect.ValueOf(c.service[k]).Pointer())

			md := reflect.TypeOf(c.objects[k])

			//if md.Kind() != reflect.Ptr {
			if !md.AssignableTo(ft) {
				fmt.Printf("field=[%s] service=[%s], name=%s\n", ft.String(), md.String(), ft.Name())
				println("not assignable to")
				continue
			}
			println("assignable to==========================================")

			// КАК понять что поле b.A является ServiceA и присвоить полю b.A ссылку на cs[k] (a)?

			//				if ft.Implements(reflect.TypeOf(&cs[k]).) == false {
			//					println("not implements")
			//				}
			//fs.SetPointer(unsafe.Pointer(reflect.ValueOf(cs[0]).Elem().Pointer()))
			//println("here")
			v := reflect.NewAt(reflect.TypeOf(c.objects[k]).Elem(), unsafe.Pointer(reflect.ValueOf(c.objects[k]).Pointer()))
			fs.Set(v)
		}
	}
}
*/
