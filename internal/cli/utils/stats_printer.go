package utils

import "fmt"

type StatsPrinter[T any] struct {
	total     int
	completed int
	succeeded int
	failed    int
	results   <-chan T
	hasError  func(T) bool
}

func NewStatsPrinter[T any](total int, results <-chan T, hasError func(T) bool) *StatsPrinter[T] {
	return &StatsPrinter[T]{
		total:    total,
		results:  results,
		hasError: hasError,
	}
}

func (s *StatsPrinter[T]) Start(done chan<- bool) {
	s.printStats()
	for range s.total {
		result, ok := <-s.results
		if !ok {
			// TODO: what actually to do, if channel is closed prematurely?
			break
		}
		s.completed++
		if s.hasError(result) {
			s.failed++
		} else {
			s.succeeded++
		}
		s.printStats()

		if s.completed == s.total {
			break
		}
	}
	fmt.Println()
	done <- true
}

func (s *StatsPrinter[T]) printStats() {
	running := s.total - s.completed
	fmt.Printf("\rRunning: %-4d | Succeeded: %-4d | Failed: %-4d | Total: %d",
		running, s.succeeded, s.failed, s.total)
}
