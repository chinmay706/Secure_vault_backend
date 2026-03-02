package services

import "log"

var backgroundSem = make(chan struct{}, 10)

// TryRunBackground attempts to run fn in a bounded goroutine pool.
// Returns true if the task was accepted, false if the pool is full.
func TryRunBackground(fn func()) bool {
	select {
	case backgroundSem <- struct{}{}:
		go func() {
			defer func() { <-backgroundSem }()
			fn()
		}()
		return true
	default:
		log.Println("[BACKGROUND] Rejected task: concurrency limit reached")
		return false
	}
}
