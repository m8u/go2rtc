package record

import (
	"fmt"
	"os"
	"sync"
	"time"
)

const dateFormat = "2006-01-02_15_04_05"

type Segments struct {
	segmentDuration time.Duration
	numSegments     int
	path            string

	mu       sync.Mutex
	files    []*os.File
	current  int
	onSwitch func(i int)
}

func NewSegments(segmentDuration time.Duration, numSegments int, path string) (segments *Segments, err error) {
	segments = &Segments{
		segmentDuration: segmentDuration,
		numSegments:     numSegments,
		path:            path,
		files:           make([]*os.File, numSegments),
	}
	err = os.MkdirAll(path, 0750)
	if err != nil {
		return nil, err
	}

	go segments.scheduleSwitch()

	return
}

func (s *Segments) scheduleSwitch() {
	s.switchFile()
	for range time.NewTicker(s.segmentDuration).C {
		s.switchFile()
		go s.onSwitch(s.current)
	}
}

func (s *Segments) switchFile() {
	var err error

	s.mu.Lock()
	if s.files[s.current] != nil {
		_ = s.files[s.current].Close()
	}
	s.current++
	if s.current == s.numSegments {
		s.current = 0
	}
	now := time.Now()
	filename := fmt.Sprintf(
		"%s/%s_%s.mp4",
		s.path,
		now.Format(dateFormat), (now.Add(s.segmentDuration)).Format(dateFormat),
	)
	if s.files[s.current] != nil {
		err = os.Remove(s.files[s.current].Name())
		if err != nil {
			log.Error().Err(err).Msg("failed to remove segment file")
		}
	}
	s.files[s.current], err = os.OpenFile(
		filename,
		os.O_CREATE|os.O_WRONLY,
		0644,
	)
	if err != nil {
		log.Error().Err(err).Msg("failed to open segment file")
	}
	s.mu.Unlock()
}

func (s *Segments) OnSwitch(f func(i int)) {
	s.onSwitch = f
}

func (s *Segments) Write(b []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.files[s.current].Write(b)
}
