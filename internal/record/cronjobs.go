package record

import (
	"fmt"
	"math"
	"os/exec"
	"time"

	"github.com/AlexxIT/go2rtc/internal/app"
)

// FFmpegFinalizeRecordings is a cron job that runs ffmpeg_finalize.py
// to fix recordings so they are playable by any player.
// i'm sorry
func FFmpegFinalizeRecordings() {
	var cfg struct {
		Record map[string]any `yaml:"record"`
	}
	app.LoadConfig(&cfg)

	recordingsBasePath := cfg.Record["basePath"].(string)

	cmd := exec.Command(cfg.Record["finalizeScriptPath"].(string), recordingsBasePath)
	log.Debug().Msgf("cron job 'FFmpegFinalizeRecordings' will execute: %s", cmd.String())

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error().Err(err).Msgf("cron job 'FFmpegFinalizeRecordings' has failed: %s", output)
		return
	}
}

// RemoveDanglingRecodings is a cron job that removes dangling recordings
// older than the configured window, e.g. caused by a crash.
func RemoveDanglingRecodings() {
	var cfg struct {
		Record map[string]any `yaml:"record"`
	}
	app.LoadConfig(&cfg)

	recordingsBasePath := cfg.Record["basePath"].(string)
	segmentDuration, _ := time.ParseDuration(cfg.Record["segmentDuration"].(string))
	numSegments := cfg.Record["numSegments"].(int)
	lifespanMins := math.Ceil(segmentDuration.Minutes()*float64(numSegments)) + 5

	cmd := exec.Command(
		"find",
		recordingsBasePath,
		"-type",
		"f",
		"-mmin",
		fmt.Sprintf("+%.0f", lifespanMins),
		"-delete",
	)
	log.Debug().Msgf("cron job 'RemoveDanglingRecodings' will execute: %s", cmd.String())

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error().Err(err).Msgf("cron job 'RemoveDanglingRecodings' has failed: %s", output)
		return
	}
}
