package cronjobs

import (
	"fmt"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/record"
	"github.com/robfig/cron"
)

func Init() {
	var cfg struct {
		Record map[string]any `yaml:"record"`
	}
	app.LoadConfig(&cfg)

	c := cron.New()
	c.Start()

	// add scheduled jobs below
	c.AddFunc(
		fmt.Sprintf("@every %s", cfg.Record["segmentDuration"].(string)),
		record.FFmpegFinalizeRecordings,
	)
	c.AddFunc("* */5 * * * *", record.RemoveDanglingRecodings)
}
