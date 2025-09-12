package config

import (
	"os"

	"gopkg.in/yaml.v2"
)

func LoadConfigFromFile(path string) (*Config, error) {
	if path == "" {
		path = "./config/config.yaml"
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	var config Config
	dec := yaml.NewDecoder(file)
	err = dec.Decode(&config)
	if err != nil {
		return nil, err
	}
	AppConfig = &config
	return &config, nil
}

type Config struct {
	SaveDir             string `yaml:"save_dir"`
	TempDir             string `yaml:"temp_dir"`
	MusicDir            string `yaml:"music_dir"`
	SpotifyClientID     string `yaml:"spotify_client_id"`
	SpotifyClientSecret string `yaml:"spotify_client_secret"`
}

var AppConfig *Config
