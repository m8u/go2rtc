package app

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/shell"
	"github.com/AlexxIT/go2rtc/pkg/yaml"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
)

var Version = "1.8.5"
var UserAgent = "go2rtc/" + Version

var ConfigPath string
var Info = map[string]any{
	"version": Version,
}

func Init() {
	var confs Config
	var version bool

	flag.Var(&confs, "config", "go2rtc config (path to file or raw text), support multiple")
	flag.BoolVar(&version, "version", false, "Print the version of the application and exit")
	flag.Parse()

	if version {
		fmt.Println("Current version: ", Version)
		os.Exit(0)
	}

	if confs == nil {
		confs = []string{"go2rtc.yaml"}
	}

	for _, conf := range confs {
		if conf[0] != '{' {
			// config as file
			if ConfigPath == "" {
				ConfigPath = conf
			}

			data, _ := os.ReadFile(conf)
			if data == nil {
				continue
			}

			data = []byte(shell.ReplaceEnvVars(string(data)))
			configs = append(configs, data)
		} else {
			// config as raw YAML
			configs = append(configs, []byte(conf))
		}
	}

	if ConfigPath != "" {
		if !filepath.IsAbs(ConfigPath) {
			if cwd, err := os.Getwd(); err == nil {
				ConfigPath = filepath.Join(cwd, ConfigPath)
			}
		}
		Info["config_path"] = ConfigPath
	}

	var cfg struct {
		Mod map[string]string `yaml:"log"`
	}

	LoadConfig(&cfg)

	go listenConfig()

	log.Logger = NewLogger(cfg.Mod["format"], cfg.Mod["level"])

	modules = cfg.Mod

	log.Info().Msgf("go2rtc version %s %s/%s", Version, runtime.GOOS, runtime.GOARCH)

	migrateStore()
}

func LoadConfig(v any) {
	for _, data := range configs {
		if err := yaml.Unmarshal(data, v); err != nil {
			log.Warn().Err(err).Msg("[app] read config")
		}
	}
}

func PatchConfig(key string, value any, path ...string) error {
	if ConfigPath == "" {
		return errors.New("config file disabled")
	}

	// empty config is OK
	b, _ := os.ReadFile(ConfigPath)

	b, err := yaml.Patch(b, key, value, path...)
	if err != nil {
		return err
	}

	return os.WriteFile(ConfigPath, b, 0644)
}

/*
	{
	  "action": <"add", "remove">,
	  "guid": "adsadasd13213",
	  "url": "rtsp://qwdjqpwdjoqd",
	  "device_name": "ул. Восход, 26/1 doorbell"
	}
*/
func listenConfig() {
	var cfg struct {
		RabbitMQ map[string]any `yaml:"rabbitmq"`
	}
	LoadConfig(&cfg)
	url, _ := cfg.RabbitMQ["url"].(string)

	conn, err := amqp.Dial(url)
	if err != nil {
		log.Error().Err(err).Msg("failed to connect to rabbitmq")
		return
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Error().Err(err).Msg("failed to open amqp channel")
		return
	}
	defer ch.Close()

	q, err := ch.QueueDeclare("config", true, false, false, false, nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to create amqp queue")
		return
	}
	msgs, err := ch.Consume(q.Name, "", true, false, false, false, nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to start amqp consumer")
		return
	}

	const (
		ADD    string = "add"
		REMOVE string = "remove"
	)
	for m := range msgs {
		log.Info().Msgf("new message from config queue: %s", m.Body)
		var msg map[string]string
		err = json.Unmarshal(m.Body, &msg)
		if err != nil {
			log.Error().Err(err).Msg("invalid JSON")
			continue
		}

		action, ok := msg["action"]
		if !ok {
			log.Error().Err(err).Msg("stream JSON must specify 'action'")
			continue
		}
		delete(msg, "action")
		guid, ok := msg["guid"]
		if !ok {
			log.Error().Err(err).Msg("stream JSON must specify 'guid'")
			continue
		}
		_, ok = msg["url"]
		if !ok {
			log.Error().Err(err).Msg("stream JSON must specify 'url'")
			continue
		}
		_, ok = msg["device_name"]
		if !ok {
			log.Error().Err(err).Msg("stream JSON must specify 'device_name'")
			continue
		}

		var updatedConfigBytes []byte
		switch action {
		case ADD:
			delete(msg, "guid")
			changes := map[string]map[string]map[string]string{
				"streams": {
					guid: msg,
				},
			}
			bytes, err := yaml.Encode(changes, 2)
			if err != nil {
				log.Error().Err(err).Msgf("failed to encode yaml:\n%v\n", changes)
				continue
			}
			updatedConfigBytes, err = MergeYAML(ConfigPath, bytes)
			if err != nil {
				log.Error().Err(err).Msgf("failed to merge yaml:\n%s\n", bytes)
				continue
			}
		case REMOVE:
			configBytes, _ := os.ReadFile(ConfigPath)
			var config map[string]map[string]any
			_ = yaml.Unmarshal(configBytes, &config)
			delete(config["streams"], guid)
			updatedConfigBytes, _ = yaml.Encode(config, 2)
		default:
		}
		if err = os.WriteFile(ConfigPath, updatedConfigBytes, 0644); err != nil {
			log.Error().Err(err).Msg("failed to save config")
			return
		}

		log.Info().Msg("successfully updated config, restarting...")
		go shell.Restart()
	}
}

// internal

type Config []string

func (c *Config) String() string {
	return strings.Join(*c, " ")
}

func (c *Config) Set(value string) error {
	*c = append(*c, value)
	return nil
}

var configs [][]byte
