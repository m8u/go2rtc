package record

import (
	"bytes"
	"fmt"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
	"os"
	"time"
)

const dateFormat = "2006-01-02_15_04_05"

var mp4MagicNumber = []byte{0, 0, 0, 28, 102, 116, 121, 112}

type Segments struct {
	segmentDuration time.Duration
	numSegments     int
	path            string

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
	if bytes.HasPrefix(b, mp4MagicNumber) {
		s.switchFile()
	}
	return s.files[s.current].Write(b)
}

func (s *Segments) Record() {
	s.prepareNextFile()

	s.cons = mp4.NewConsumer(s.medias)
	if err := s.stream.AddConsumer(s.cons); err != nil {
		log.Error().Err(err).Msgf("failed to add a recording consumer (%s)", s.streamName)
	}
	go func() {
		_, _ = s.cons.WriteTo(s) // blocks
	}()

	s.scheduleSwitch()
}

func (s *Segments) switchFile() {
	prev := s.current
	s.current++
	if s.current == s.numSegments {
		s.current = 0
	}
	go func() {
		if s.files[prev] != nil {
			_ = s.files[prev].Close()
		}
	}()
}

func (s *Segments) prepareNextFile() {
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
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644,
	)
	if err != nil {
		log.Error().Err(err).Msg("failed to open new segment file")
	}

	next := s.current + 1
	if next == s.numSegments {
		next = 0
	}
	if s.files[next] != nil {
		oldFilename := s.files[next].Name()
		go func() {
			err = os.Remove(oldFilename)
			if err != nil {
				log.Error().Err(err).Msg("failed to remove old segment file")
			}
		}()
	}
	s.files[next] = newFile
}

func (s *Segments) scheduleSwitch() {
	for range time.NewTicker(s.segmentDuration).C {
		s.prepareNextFile()
		s.cons.ResetMuxer() // trigger the muxer to send mp4 magic number
	}
}
