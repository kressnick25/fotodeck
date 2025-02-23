package images_test

import (
	"album/internal/images"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	copyDirectory(dataPath, homePath)

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
	files := must(loader.LoadOriginals(homePath))

	// THEN
	if len(files) != numJpgFiles {
		t.Errorf("Expected 2 loaded files but got %d", len(files))
	}
}

func TestLoaderOptimise(t *testing.T) {
	loader, teardown := setupTest(t)
	defer teardown(t)

	// GIVEN
	files := must(loader.LoadOriginals(homePath))
	assert.Equal(t, len(files), numJpgFiles, "Loaded files should equal the number on disk")
	numJpgs := must(countFilesByExtension(homePath, "jpg"))
	assert.Equal(t, numJpgs, numJpgFiles, "There should be the same number of files on disk as loaded")

	// WHEN
	err := loader.OptimiseImages(&files)
	if err != nil {
		t.Error(err)
	}

	// THEN
	assert.Equal(t, len(files), numJpgFiles, "Loaded files should equal the number on disk")
	assert.Equal(t, must(countFilesByExtension(homePath, optExt+".jpg")), numJpgFiles, "Optimised image files should be created")
	assert.Equal(t, must(countFilesByExtension(homePath, prevExt+".jpg")), numJpgFiles, "Preview image files should be created")
}

func TestLoaderReload(t *testing.T) {
	loader, teardown := setupTest(t)
	defer teardown(t)

	// GIVEN
	files := must(loader.LoadOriginals(homePath))
	err := loader.OptimiseImages(&files)
	if err != nil {
		t.Error(err)
	}

	assert.Equal(t, len(files), numJpgFiles, "Loaded files should equal the number on disk")
	assert.Equal(t, must(countFilesByExtension(homePath, optExt+".jpg")), numJpgFiles, "Optimised image files should be created")
	assert.Equal(t, must(countFilesByExtension(homePath, prevExt+".jpg")), numJpgFiles, "Preview image files should be created")

	// WHEN
	files2 := must(loader.Reload(homePath))

	// THEN
	assert.Equal(t, len(files), len(files2), "Loaded files should be the same after reload")
	assert.Equal(t, must(countFilesByExtension(homePath, optExt+".jpg")), numJpgFiles, "Optimised image files should be created")
	assert.Equal(t, must(countFilesByExtension(homePath, prevExt+".jpg")), numJpgFiles, "Preview image files should be created")
}

func TestImageCleanup(t *testing.T) {
	loader, teardown := setupTest(t)
	defer teardown(t)

	// GIVEN
	files := must(loader.LoadOriginals(homePath))
	err := loader.OptimiseImages(&files)
	if err != nil {
		t.Error(err)
	}

	assert.Equal(t, len(files), numJpgFiles, "Loaded files should equal the number on disk")
	assert.Equal(t, must(countFilesByExtension(homePath, optExt+".jpg")), numJpgFiles, "Optimised image files should be created")
	assert.Equal(t, must(countFilesByExtension(homePath, prevExt+".jpg")), numJpgFiles, "Preview image files should be created")

	// WHEN
	for _, v := range files {
		err := v.Cleanup()
		if err != nil {
			t.Error(err)
		}
	}

	// THEN
	assert.Equal(t, must(countFilesByExtension(homePath, optExt+".jpg")), 0, "Optimised image files should be created")
	assert.Equal(t, must(countFilesByExtension(homePath, prevExt+".jpg")), 0, "Preview image files should be created")
}

func must[T any](obj T, err error) T {
	if err != nil {
		panic(err)
	}
	return obj
}

// does not handle recursion
func countFilesByExtension(dir, ext string) (int, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ext) {
			count++
		}
	}
	return count, nil
}

func copyDirectory(src string, dst string) error {
	err := os.MkdirAll(dst, os.ModePerm)
	if err != nil {
		return err
	}

	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get the relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		if info.IsDir() {
			// Create sub-directories
			return os.MkdirAll(filepath.Join(dst, relPath), info.Mode())
		}

		// Copy files
		return copyFile(path, filepath.Join(dst, relPath))
	})
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// Create the destination file
	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	// Copy the contents
	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	// Copy the mode/permissions
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dst, sourceInfo.Mode())
}
