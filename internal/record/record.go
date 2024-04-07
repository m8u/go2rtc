package record

import (
	"fmt"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
	"github.com/rs/zerolog"
	"sync"
	"time"
)

var log zerolog.Logger
var recordings = map[string]*Segments{}

func Init() {
	log = app.GetLogger("record")

	var cfg struct {
		Record  map[string]any `yaml:"record"`
		Streams map[string]any `yaml:"streams"`
	}

	// todo defaults

	app.LoadConfig(&cfg)

	basePath, ok := cfg.Record["basePath"].(string)
	if !ok {
		log.Fatal().Msg("record.basePath is invalid")
	}

	segmentDurationStr, ok := cfg.Record["segmentDuration"].(string)
	segmentDuration, err := time.ParseDuration(segmentDurationStr)
	if !ok || err != nil {
		log.Fatal().Msg("record.segmentDuration is invalid")
	}

	numSegments, ok := cfg.Record["numSegments"].(int)
	if !ok {
		log.Fatal().Msg("record.numSegments is invalid")
	}

	for name, item := range cfg.Streams {
		switch item.(type) {
		case map[string]any:
			seg, err := NewSegments(segmentDuration, numSegments, fmt.Sprintf("%s/%s", basePath, name))
			if err != nil {
				log.Fatal().Err(err).Msg("failed to create segments")
			}
			recordings[name] = seg
			go record(name, seg)
		default:
			continue
		}
	}
}

func record(name string, seg *Segments) {
	stream := streams.Get(name)

	medias := mp4.ParseQuery(map[string][]string{"src": {name}, "mp4": {"all"}})
	var cons *mp4.Consumer
	var mu sync.Mutex

	reset := func(i int) {
		cons = mp4.NewConsumer(medias)
		if err := stream.AddConsumer(cons); err != nil {
			log.Error().Err(err).Msgf("failed to add a recording consumer (%s)", name)
		}
		mu.Lock()
		log.Debug().Msgf("writing to segment #%d (%s)", i, name)
		_, _ = cons.WriteTo(seg)

		stream.RemoveConsumer(cons)
		mu.Unlock()
	}

	seg.OnSwitch(func(i int) {
		_ = cons.Stop()
		mu.Lock()
		go reset(i)
		mu.Unlock()
	})

	go reset(0)
}
