package probing

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"
)

var (
	ErrNotFound = errors.New("probing: id not found")
	ErrExist    = errors.New("probing: id exists")
)

type Prober interface {
	AddHTTP(id string, probingInterval time.Duration, endpoint string) error
	Remove(id string) error
	Reset(id string) error
	Status(id string) (Status, error)
}

type prober struct {
	mu      sync.Mutex
	targets map[string]*status
}

func NewProber() Prober {
	return &prober{targets: make(map[string]*status)}
}

func (p *prober) AddHTTP(id string, probingInterval time.Duration, endpoint string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.targets[id]; ok {
		return ErrExist
	}

	s := &status{stopC: make(chan struct{})}
	p.targets[id] = s

	ticker := time.NewTicker(probingInterval)

	go func() {
		for {
			select {
			case <-ticker.C:
				start := time.Now()
				resp, err := http.Get(endpoint)

				if err != nil {
					s.recordFailure()
					continue
				}

				var hh Health
				d := json.NewDecoder(resp.Body)
				err = d.Decode(&hh)
				resp.Body.Close()
				if err != nil || !hh.OK {
					s.recordFailure()
					continue
				}

				s.record(time.Since(start), hh.Now)
			case <-s.stopC:
				ticker.Stop()
				return
			}
		}
	}()

	return nil
}

func (p *prober) Remove(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	s, ok := p.targets[id]
	if !ok {
		return ErrNotFound
	}
	close(s.stopC)
	delete(p.targets, id)
	return nil
}

func (p *prober) Reset(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	s, ok := p.targets[id]
	if !ok {
		return ErrNotFound
	}
	s.reset()
	return nil
}

func (p *prober) Status(id string) (Status, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	s, ok := p.targets[id]
	if !ok {
		return nil, ErrNotFound
	}
	return s, nil
}
