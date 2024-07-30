package record

import (
	"fmt"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
	"os"
	"sync"
	"time"
)

const dateFormat = "2006-01-02_15_04_05"

type Segments struct {
	segmentDuration time.Duration
	numSegments     int
	path            string

	mu      sync.Mutex
	files   []*os.File
	current int

	streamName string
	stream     *streams.Stream
	medias     []*core.Media
	cons       *mp4.Consumer
}

func NewSegments(segmentDuration time.Duration, numSegments int, path string, streamName string) (segments *Segments, err error) {
	segments = &Segments{
		segmentDuration: segmentDuration,
		numSegments:     numSegments,
		path:            path,
		files:           make([]*os.File, numSegments),
		streamName:      streamName,
		stream:          streams.Get(streamName),
		medias:          mp4.ParseQuery(map[string][]string{"src": {streamName}, "mp4": {"all"}}),
	}
	err = os.MkdirAll(path, 0750)
	if err != nil {
		return nil, err
	}

	return
}

func (s *Segments) Write(b []byte) (n int, err error) {
	if ok := s.mu.TryLock(); ok {
		defer s.mu.Unlock()
		return s.files[s.current].Write(b)
	} else {
		// files are switching, wait and then write to previous
		s.mu.Lock()
		defer s.mu.Unlock()
		prev := s.current - 1
		if prev == -1 {
			prev = s.numSegments - 1
		}
		return s.files[prev].Write(b)
	}
}

func (s *Segments) Record() {
	s.switchFile()

	s.cons = mp4.NewConsumer(s.medias)
	if err := s.stream.AddConsumer(s.cons); err != nil {
		log.Error().Err(err).Msgf("failed to add a recording consumer (%s)", s.streamName)
	}
	go func() {
		_, _ = s.cons.WriteTo(s) // blocks
	}()

	go s.scheduleSwitch()
}

func (s *Segments) switchFile() {
	var err error

	now := time.Now()
	filename := fmt.Sprintf(
		"%s/%s_%s.mp4",
		s.path,
		now.Format(dateFormat),
		now.Add(s.segmentDuration).Format(dateFormat),
	)
	newFile, err := os.OpenFile(
		filename,
		os.O_CREATE|os.O_WRONLY,
		0644,
	)
	if err != nil {
		log.Error().Err(err).Msg("failed to open segment file")
	}

	s.mu.Lock()

	if s.files[s.current] != nil {
		_ = s.files[s.current].Close()
	}
	s.current++
	if s.current == s.numSegments {
		s.current = 0
	}
	if s.files[s.current] != nil {
		err = os.Remove(s.files[s.current].Name())
		if err != nil {
			log.Error().Err(err).Msg("failed to remove segment file")
		}
	}
	s.files[s.current] = newFile

	if s.cons != nil {
		s.cons.ResetMuxer()
	}

	s.mu.Unlock()
}

func (s *Segments) scheduleSwitch() {
	for range time.NewTicker(s.segmentDuration).C {
		go s.switchFile()
	}
}
