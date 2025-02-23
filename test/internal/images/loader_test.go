package images_test

import (
	"album/internal/images"
	"album/internal/util"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const maxSize = 200
const optExt = "opt"
const prevExt = "prev"
const homePath = "../../workdir"
const dataPath = "../../data"
const numJpgFiles = 2

func setupTest(t *testing.T) (images.Loader, func(t *testing.T)) {
	os.RemoveAll(homePath)
	os.Mkdir(homePath, os.ModeDir)
	util.CopyDirectory(dataPath, homePath)

	defaultSize := images.Dimensions{
		Width:  maxSize,
		Height: maxSize,
	}
	loader := images.Loader{
		OptimisedExtension:     optExt,
		PreviewExtension:       prevExt,
		MaxOptimisedDimensions: defaultSize,
		MaxPreviewDimensions:   defaultSize,
	}

	return loader, func(t *testing.T) {}
}

func TestLoaderOriginals(t *testing.T) {
	loader, teardown := setupTest(t)
	defer teardown(t)

	// GIVEN

	// WHEN
	files := util.Must(loader.LoadOriginals(homePath))

	// THEN
	if len(files) != numJpgFiles {
		t.Errorf("Expected 2 loaded files but got %d", len(files))
	}
}

func TestLoaderOptimise(t *testing.T) {
	loader, teardown := setupTest(t)
	defer teardown(t)

	// GIVEN
	files := util.Must(loader.LoadOriginals(homePath))
	assert.Equal(t, len(files), numJpgFiles, "Loaded files should equal the number on disk")
	numJpgs := util.Must(util.CountFilesByExtension(homePath, "jpg"))
	assert.Equal(t, numJpgs, numJpgFiles, "There should be the same number of files on disk as loaded")
	for _, file := range files {
		assert.False(t, file.IsOptimised(), "Files should not be optimised before Optimise func")
		assert.Equal(t, file.GetPreview(), file.GetFullSize(), "Preview should return full size path before optimisation")
	}

	// WHEN
	err := loader.OptimiseImages(&files)
	if err != nil {
		t.Error(err)
	}

	// THEN
	assert.Equal(t, len(files), numJpgFiles, "Loaded files should equal the number on disk")
	assert.Equal(t, util.Must(util.CountFilesByExtension(homePath, optExt+".jpg")), numJpgFiles, "Optimised image files should be created")
	assert.Equal(t, util.Must(util.CountFilesByExtension(homePath, prevExt+".jpg")), numJpgFiles, "Preview image files should be created")

	for _, file := range files {
		assert.True(t, file.IsOptimised(), "Files should be optimised")
		assert.NotEqual(t, file.GetPreview(), file.GetFullSize(), "Preview should not return fullSize path after optimisation")
	}
}

func TestLoaderReload(t *testing.T) {
	loader, teardown := setupTest(t)
	defer teardown(t)

	// GIVEN
	files := util.Must(loader.LoadOriginals(homePath))
	err := loader.OptimiseImages(&files)
	if err != nil {
		t.Error(err)
	}

	assert.Equal(t, len(files), numJpgFiles, "Loaded files should equal the number on disk")
	assert.Equal(t, util.Must(util.CountFilesByExtension(homePath, optExt+".jpg")), numJpgFiles, "Optimised image files should be created")
	assert.Equal(t, util.Must(util.CountFilesByExtension(homePath, prevExt+".jpg")), numJpgFiles, "Preview image files should be created")

	// WHEN
	files2 := util.Must(loader.Reload(homePath))

	// THEN
	assert.Equal(t, len(files), len(files2), "Loaded files should be the same after reload")
	assert.Equal(t, util.Must(util.CountFilesByExtension(homePath, optExt+".jpg")), numJpgFiles, "Optimised image files should be created")
	assert.Equal(t, util.Must(util.CountFilesByExtension(homePath, prevExt+".jpg")), numJpgFiles, "Preview image files should be created")
}

func TestImageCleanup(t *testing.T) {
	loader, teardown := setupTest(t)
	defer teardown(t)

	// GIVEN
	files := util.Must(loader.LoadOriginals(homePath))
	err := loader.OptimiseImages(&files)
	if err != nil {
		t.Error(err)
	}

	assert.Equal(t, len(files), numJpgFiles, "Loaded files should equal the number on disk")
	assert.Equal(t, util.Must(util.CountFilesByExtension(homePath, optExt+".jpg")), numJpgFiles, "Optimised image files should be created")
	assert.Equal(t, util.Must(util.CountFilesByExtension(homePath, prevExt+".jpg")), numJpgFiles, "Preview image files should be created")

	// WHEN
	for _, v := range files {
		err := v.Cleanup()
		if err != nil {
			t.Error(err)
		}
	}

	// THEN
	assert.Equal(t, util.Must(util.CountFilesByExtension(homePath, optExt+".jpg")), 0, "Optimised image files should be created")
	assert.Equal(t, util.Must(util.CountFilesByExtension(homePath, prevExt+".jpg")), 0, "Preview image files should be created")
}
