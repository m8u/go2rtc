package app

import (
	"encoding/json"
	"errors"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/shell"
	"github.com/AlexxIT/go2rtc/pkg/yaml"
)

func LoadConfig(v any) {
	for _, data := range configs {
		if err := yaml.Unmarshal(data, v); err != nil {
			Logger.Warn().Err(err).Send()
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

type flagConfig []string

func (c *flagConfig) String() string {
	return strings.Join(*c, " ")
}

func (c *flagConfig) Set(value string) error {
	*c = append(*c, value)
	return nil
}

var configs [][]byte

func initConfig(confs flagConfig) {
	if confs == nil {
		confs = []string{"go2rtc.yaml"}
	}

	for _, conf := range confs {
		if len(conf) == 0 {
			continue
		}
		if conf[0] == '{' {
			// config as raw YAML or JSON
			configs = append(configs, []byte(conf))
		} else if data := parseConfString(conf); data != nil {
			configs = append(configs, data)
		} else {
			// config as file
			if ConfigPath == "" {
				ConfigPath = conf
			}

			if data, _ = os.ReadFile(conf); data == nil {
				continue
			}

			data = []byte(shell.ReplaceEnvVars(string(data)))
			configs = append(configs, data)
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
}

func parseConfString(s string) []byte {
	i := strings.IndexByte(s, '=')
	if i < 0 {
		return nil
	}

	items := strings.Split(s[:i], ".")
	if len(items) < 2 {
		return nil
	}

	// `log.level=trace` => `{log: {level: trace}}`
	var pre string
	var suf = s[i+1:]
	for _, item := range items {
		pre += "{" + item + ": "
		suf += "}"
	}

	return []byte(pre + suf)
}

/*
	{
	  "action": <"add", "remove">,
	  "guid": "adsadasd13213",
	  "url": "rtsp://qwdjqpwdjoqd",
	  "device_name": "ул. Восход, 26/1 doorbell"
	}
*/
func ListenConfig() {
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
		shell.Restart()
	}
}

func MergeYAML(file1 string, yaml2 []byte) ([]byte, error) {
	// Read the contents of the first YAML file
	data1, err := os.ReadFile(file1)
	if err != nil {
		return nil, err
	}

	// Unmarshal the first YAML file into a map
	var config1 map[string]any
	if err = yaml.Unmarshal(data1, &config1); err != nil {
		return nil, err
	}

	// Unmarshal the second YAML document into a map
	var config2 map[string]any
	if err = yaml.Unmarshal(yaml2, &config2); err != nil {
		return nil, err
	}

	// Merge the two maps
	config1 = merge(config1, config2)

	// Marshal the merged map into YAML
	return yaml.Encode(&config1, 2)
}

func merge(dst, src map[string]any) map[string]any {
	for k, v := range src {
		if vv, ok := dst[k]; ok {
			switch vv := vv.(type) {
			case map[string]any:
				v := v.(map[string]any)
				dst[k] = merge(vv, v)
			case []any:
				v := v.([]any)
				dst[k] = v
			default:
				dst[k] = v
			}
		} else {
			dst[k] = v
		}
	}
	return dst
}
