package config_test

import (
	"album/internal/application"
	"album/internal/util"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const configPath = "../../../config.toml"
const homePath = "./workdir"

func setup() func() {
	os.MkdirAll(homePath, os.FileMode(0755))
	return func() {
		os.RemoveAll(homePath)
	}
}

func TestValidConfig(t *testing.T) {
	util.Must(application.LoadConfig(configPath))
}

func TestValidateHomePath(t *testing.T) {
	teardown := setup()
	defer teardown()

	config := util.Must(application.LoadConfig(configPath))
	config.Home.Path = homePath

	err := application.ValidateHomePath(config)

	assert.Nil(t, err, "Error was returned on valid home path")
}

func TestValidateHomePathMissingPath(t *testing.T) {
	config := util.Must(application.LoadConfig(configPath))
	config.Home.Path = "./tmp/does-not-exist"

	err := application.ValidateHomePath(config)

	assert.ErrorContains(t, err, "home path does not exist")
}

func TestValidateHomePathNonDir(t *testing.T) {
	config := util.Must(application.LoadConfig(configPath))
	config.Home.Path = "config_test.go"

	err := application.ValidateHomePath(config)

	assert.ErrorContains(t, err, "home path is not a directory")
}
