package lsp

import (
	"sync"
	"time"

	"github.com/type-rb/type-rb/internal/compilerservice"
)

const diagnosticDebounce = 150 * time.Millisecond
const diagnosticShutdownGrace = 100 * time.Millisecond

type diagnosticRequest struct {
	generation uint64
	targets    map[string]bool
	versions   map[string]int
}

type diagnosticResult struct {
	request  diagnosticRequest
	snapshot compilerservice.Snapshot
}

// diagnosticCoordinator debounces full-project analysis without owning any
// editor or protocol state. The server event loop is the sole consumer of
// completed results and remains responsible for applying snapshots and
// publishing diagnostics.
type diagnosticCoordinator struct {
	delay      time.Duration
	analyze    func() (compilerservice.Snapshot, bool)
	generation func() uint64
	requests   chan diagnosticRequest
	results    chan diagnosticResult
	stop       chan struct{}
	done       chan struct{}
	started    chan struct{}
	startOnce  sync.Once
	stopOnce   sync.Once
}

func newDiagnosticCoordinator(
	delay time.Duration,
	analyze func() (compilerservice.Snapshot, bool),
	generation func() uint64,
) *diagnosticCoordinator {
	return &diagnosticCoordinator{
		delay: delay, analyze: analyze, generation: generation,
		requests: make(chan diagnosticRequest, 1),
		results:  make(chan diagnosticResult, 1),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
		started:  make(chan struct{}),
	}
}

func (c *diagnosticCoordinator) Start() {
	if c == nil {
		return
	}
	c.startOnce.Do(func() {
		close(c.started)
		go c.run()
	})
}

func (c *diagnosticCoordinator) Stop(grace time.Duration) {
	if c == nil {
		return
	}
	c.stopOnce.Do(func() { close(c.stop) })
	select {
	case <-c.started:
	case <-c.done:
		return
	default:
		return
	}
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-c.done:
	case <-timer.C:
	}
}

func (c *diagnosticCoordinator) Request(request diagnosticRequest) {
	if c == nil {
		return
	}
	select {
	case <-c.stop:
		return
	default:
	}
	select {
	case c.requests <- request:
		return
	default:
	}
	select {
	case <-c.requests:
	default:
	}
	select {
	case c.requests <- request:
	case <-c.stop:
	}
}

func (c *diagnosticCoordinator) Results() <-chan diagnosticResult {
	if c == nil {
		return nil
	}
	return c.results
}

func (c *diagnosticCoordinator) run() {
	defer close(c.done)
	defer close(c.results)
	var timer *time.Timer
	var timerChannel <-chan time.Time
	var pending *diagnosticRequest
	schedule := func(request diagnosticRequest) {
		pending = &request
		if timer == nil {
			timer = time.NewTimer(c.delay)
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(c.delay)
		}
		timerChannel = timer.C
	}
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	for {
		select {
		case <-c.stop:
			return
		case request := <-c.requests:
			schedule(request)
		case <-timerChannel:
			timerChannel = nil
			select {
			case request := <-c.requests:
				schedule(request)
				continue
			default:
			}
			if pending == nil {
				continue
			}
			request := *pending
			pending = nil
			snapshot, current := c.analyze()
			if !current || snapshot.Version != request.generation || c.generation() != request.generation {
				continue
			}
			result := diagnosticResult{request: request, snapshot: snapshot}
			select {
			case c.results <- result:
			case <-c.stop:
				return
			}
		}
	}
}
