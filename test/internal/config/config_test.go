package config_test

import (
	"album/internal/application"
	"album/internal/util"
	"testing"
)

const configPath = "../../../config.toml"

func TestValidConfig(t *testing.T) {
	util.Must(application.LoadConfig(configPath))
}
