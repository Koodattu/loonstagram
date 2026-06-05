package httpx

import "sync"

type mediaFlight struct {
	mu    sync.Mutex
	calls map[string]*mediaFlightCall
}

type mediaFlightCall struct {
	wg  sync.WaitGroup
	err error
}

func newMediaFlight() *mediaFlight {
	return &mediaFlight{calls: make(map[string]*mediaFlightCall)}
}

func (f *mediaFlight) Do(key string, fn func() error) error {
	f.mu.Lock()
	if call, ok := f.calls[key]; ok {
		f.mu.Unlock()
		call.wg.Wait()
		return call.err
	}

	call := &mediaFlightCall{}
	call.wg.Add(1)
	f.calls[key] = call
	f.mu.Unlock()

	call.err = fn()
	call.wg.Done()

	f.mu.Lock()
	delete(f.calls, key)
	f.mu.Unlock()

	return call.err
}
