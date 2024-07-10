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

func (s *Segments) Record() {
	go s.switchFile()
	go s.scheduleSwitch()
}

func (s *Segments) switchFile() {
	var err error
	var wg sync.WaitGroup

	newCons := mp4.NewConsumer(s.medias)

	wg.Add(1)
	go func() {
		if err := s.stream.AddConsumer(newCons); err != nil {
			log.Error().Err(err).Msgf("failed to add a recording consumer (%s)", s.streamName)
		}
		wg.Done()
	}()

	now := time.Now()
	filename := fmt.Sprintf(
		"%s/%s_%s.mp4",
		s.path,
		now.Format(dateFormat), (now.Add(s.segmentDuration)).Format(dateFormat),
	)
	newFile, err := os.OpenFile(
		filename,
		os.O_CREATE|os.O_WRONLY,
		0644,
	)
	if err != nil {
		log.Error().Err(err).Msg("failed to open segment file")
	}

	wg.Wait()

	go func() {
		_, _ = newCons.WriteTo(newFile) // blocks
	}()

	if s.cons != nil {
		time.Sleep(time.Second * 3) // write to prev segment for a few extra seconds just in case
		_ = s.cons.Stop()
		s.stream.RemoveConsumer(s.cons) // todo potential gc malfunction here
	}
	s.cons = newCons

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
}

func (s *Segments) scheduleSwitch() {
	for range time.NewTicker(s.segmentDuration).C {
		go s.switchFile()
	}
}
