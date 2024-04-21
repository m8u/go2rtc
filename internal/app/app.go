package app

import (
	"errors"
	"flag"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/shell"
	"github.com/AlexxIT/go2rtc/pkg/yaml"
	"github.com/go-redis/redis"
	"github.com/rs/zerolog/log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

	go listenRedis()

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
	  "guid": "adsadasd13213",
	  "url": "rtsp://qwdjqpwdjoqd",
	  "record": true
	}
*/
func listenRedis() {
	var cfg struct {
		Redis map[string]any `yaml:"redis"`
	}
	LoadConfig(&cfg)

	addr, _ := cfg.Redis["addr"].(string)
	password, _ := cfg.Redis["password"].(string)
	db, _ := cfg.Redis["db"].(int)
	stream, _ := cfg.Redis["stream"].(string)

	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	const (
		ADD    string = "add"
		REMOVE        = "remove"
	)

	for {
		res, err := rdb.XRead(&redis.XReadArgs{
			Streams: []string{stream, "$"},
			Count:   1,
			Block:   0,
		}).Result()
		if err != nil {
			log.Error().Err(err).Msg("failed to read from config stream")
			return
		}
		id := res[0].Messages[0].ID
		msg := res[0].Messages[0].Values
		log.Info().Msgf("new message from config stream: %s", msg)

		action := msg["action"].(string)
		guid := msg["guid"].(string)

		var updatedConfigBytes []byte
		switch action {
		case ADD:
			delete(msg, "guid")
			changes := map[string]map[string]map[string]any{
				"streams": {
					guid: msg,
				},
			}
			bytes, err := yaml.Encode(changes, 2)
			if err != nil {
				log.Error().Err(err).Msgf("failed to encode yaml:\n%v\n", changes)
				rdb.XDel(stream, id)
				continue
			}
			updatedConfigBytes, err = MergeYAML(ConfigPath, bytes)
			if err != nil {
				log.Error().Err(err).Msgf("failed to merge yaml:\n%s\n", bytes)
				rdb.XDel(stream, id)
				continue
			}
			break
		case REMOVE:
			configBytes, _ := os.ReadFile(ConfigPath)
			var config map[string]map[string]any
			_ = yaml.Unmarshal(configBytes, &config)
			delete(config["streams"], guid)
			updatedConfigBytes, _ = yaml.Encode(config, 2)
			break
		default:
			break
		}
		if err = os.WriteFile(ConfigPath, updatedConfigBytes, 0644); err != nil {
			log.Error().Err(err).Msg("failed to save config")
			return
		}

		rdb.XDel(stream, id)

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
