package httpx

import (
	"sync"

	"instafix/internal/instagram"
)

type flight struct {
	mu    sync.Mutex
	calls map[string]*flightCall
}

type flightCall struct {
	wg   sync.WaitGroup
	post *instagram.Post
	err  error
}

func newFlight() *flight {
	return &flight{calls: make(map[string]*flightCall)}
}

func (f *flight) Do(key string, fn func() (*instagram.Post, error)) (*instagram.Post, error) {
	f.mu.Lock()
	if call, ok := f.calls[key]; ok {
		f.mu.Unlock()
		call.wg.Wait()
		return call.post, call.err
	}

	call := &flightCall{}
	call.wg.Add(1)
	f.calls[key] = call
	f.mu.Unlock()

	call.post, call.err = fn()
	call.wg.Done()

	f.mu.Lock()
	delete(f.calls, key)
	f.mu.Unlock()

	return call.post, call.err
}
