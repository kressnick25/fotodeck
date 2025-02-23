package application

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type (
	Config struct {
		Home          home
		Server        server
		ImageResizing imageResizing
	}

	imageResizing struct {
		Async                bool
		CleanupOnShutdown    bool
		Enabled              bool
		PreviewWidth         int
		PreviewHeight        int
		ResizedWidth         int
		ResizedHeight        int
		ResizedFileExtension string
		PreviewFileExtension string
	}

	server struct {
		ListenAddr string
	}

	home struct {
		Path               string
		MinRefreshInterval int
	}
)

func LoadConfig(path string) (Config, error) {
	configPath := filepath.Clean(path)

	configStat, err := os.Stat(configPath)
	if errors.Is(err, os.ErrNotExist) {
		slog.Error("path does not exist", "path", configPath)
		return Config{}, fmt.Errorf("path does not exist: %s", path)
	}
	if configStat.IsDir() {
		return Config{}, fmt.Errorf("config path is a directory: %s", path)
	}

	configFileBytes, err := os.ReadFile(configPath)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read config file '%s': %w", configPath, err)
	}
	configFileString := string(configFileBytes)

	var conf Config
	_, err = toml.Decode(configFileString, &conf)
	if err != nil {
		return Config{}, fmt.Errorf("TOML decode failed: %w", err)
	}
	slog.Info("Loaded config file", "path", configPath)

	homePath := filepath.Clean(conf.Home.Path)
	s, err := os.Stat(conf.Home.Path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("home path does not exist: %s", homePath)
	}
	if !s.IsDir() {
		return Config{}, fmt.Errorf("home path is not a directory: %s", homePath)
	}
	conf.Home.Path = homePath

	return conf, nil
}
