package utils

import (
	"fmt"

	"github.com/bluegradienthorizon/proxytoolbox/testers"
)

type StatsPrinter struct {
	total     int
	completed int
	succeeded int
	failed    int
	results   <-chan testers.LatencyTestResult
}

func NewStatsPrinter(total int, results <-chan testers.LatencyTestResult) *StatsPrinter {
	return &StatsPrinter{
		total:   total,
		results: results,
	}
}

func (s *StatsPrinter) Start(done chan<- bool) {
	s.printStats()
	for range s.total {
		result := <-s.results
		s.completed++
		if result.Error == nil {
			s.succeeded++
		} else {
			s.failed++
		}
		s.printStats()

		if s.completed == s.total {
			break
		}
	}
	fmt.Println()
	done <- true
}

func (s *StatsPrinter) printStats() {
	running := s.total - s.completed
	fmt.Printf("\rRunning: %-4d | Succeeded: %-4d | Failed: %-4d | Total: %d",
		running, s.succeeded, s.failed, s.total)
}
