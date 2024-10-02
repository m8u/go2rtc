package record

import (
	"fmt"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/rs/zerolog"
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

	for streamName, item := range cfg.Streams {
		switch item := item.(type) {
		case map[string]any:
			deviceName, ok := item["device_name"].(string)
			if !ok {
				continue
			}
			deviceName = strings.ReplaceAll(deviceName, "/", "-")
			gateAddress := strings.Split(deviceName, " (")[0] // встретимся как попадется адрес со скобками
			seg, err := NewSegments(
				segmentDuration,
				numSegments,
				fmt.Sprintf("%s/%s/%s", basePath, gateAddress, deviceName),
				streamName,
			)
			if err != nil {
				log.Fatal().Err(err).Msg("failed to create segments")
			}
			recordings[streamName] = seg

			go seg.Record()
			time.Sleep(time.Second * 2) // sleep couple seconds so streams won't switch segments all at the same time
		default:
			continue
		}
	}
}
