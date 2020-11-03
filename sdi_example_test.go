package sdi_test

import (
	"sync"
	"testing"
)

func ExampleMain() {

	type URLStorage struct {
		mux sync.RWMutex
		url []string
	}


	func (us *URLStorage) Add(u string) {
		us.mux.Lock()
		defer us.mux.Unlock()
		us.url = append(us.url, u)
	}

	func (us *URLStorage)Init(ctx context.Context ) error {
		
		return nil
	}


	type HealthChecker struct {
		
	}
	
	type Notifier struct {
		
	}
	


	_ = sdi.New()

}
