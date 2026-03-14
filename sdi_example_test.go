package sdi_test

import (
	"context"
	"fmt"
	"sync"

	"github.com/axkit/sdi"
)

type URLStorageI interface {
	Add(string)
}

type URLStorage struct {
	mux sync.RWMutex
	url []string
}

func (us *URLStorage) Add(u string) {
	us.mux.Lock()
	defer us.mux.Unlock()
	us.url = append(us.url, u)
}

func (us *URLStorage) Init(_ context.Context) error { return nil }
func (us *URLStorage) Start(_ context.Context) error { return nil }

type HealthChecker struct {
	Storage URLStorageI
}

func (hc *HealthChecker) Init(ctx context.Context) error {
	hc.Storage.Add("http://localhost")
	return nil
}

func (hc *HealthChecker) Start(ctx context.Context) error {
	return nil
}

func ExampleNew_implicit() {
	cs := sdi.New() // Implicit mode (default)

	storage := &URLStorage{}
	checker := &HealthChecker{}

	cs.Add(storage, checker)
	cs.BuildDependencies()

	if err := cs.InitRequired(context.Background()); err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println("urls:", len(storage.url))
	// Output: urls: 1
}

func ExampleNew_explicit() {
	cs := sdi.New(sdi.Explicit()) // Explicit mode — only tagged fields wired

	storage := &URLStorage{}
	checker := &HealthChecker{} // Storage field has no `sdi:"inject"` tag, so not wired

	cs.Add(storage)
	cs.Add(checker)
	cs.BuildDependencies()

	fmt.Println("wired:", checker.Storage != nil)
	// Output: wired: false
}
