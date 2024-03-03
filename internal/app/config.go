package app

import (
	"gopkg.in/yaml.v3"
	"os"
)

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
	return yaml.Marshal(&config1)
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
