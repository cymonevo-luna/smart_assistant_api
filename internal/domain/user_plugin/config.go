package userplugin

import (
	"encoding/json"
)

// ParseInstallConfig decodes a UserPlugin.Config map into InstallConfig.
func ParseInstallConfig(m map[string]any) (InstallConfig, error) {
	if m == nil {
		return InstallConfig{}, nil
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return InstallConfig{}, err
	}
	var cfg InstallConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return InstallConfig{}, err
	}
	return cfg, nil
}

// ToMap encodes InstallConfig for storage in UserPlugin.Config.
func (c InstallConfig) ToMap() (map[string]any, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}
