package task

import "sync"

type registrationAttemptCounter struct {
	mu    sync.Mutex
	limit int
	count int
}

func newRegistrationAttemptCounter(limit int) *registrationAttemptCounter {
	if limit < 0 {
		limit = 0
	}
	return &registrationAttemptCounter{limit: limit}
}

func (c *registrationAttemptCounter) reserve() (int, bool) {
	if c == nil {
		return 0, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.count >= c.limit {
		return 0, false
	}
	idx := c.count
	c.count++
	return idx, true
}

func (c *registrationAttemptCounter) done() bool {
	if c == nil {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count >= c.limit
}
