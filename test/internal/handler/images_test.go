package handler_test

import (
	"fotodeck/internal/handler"
	"fotodeck/internal/images"
	"fotodeck/internal/util"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const maxSize = 200
const optExt = "opt"
const prevExt = "prev"
const homePath = "./workdir"
const dataPath = "../../data"

func setupTest(t *testing.T) (map[string]images.ImageFile, func(t *testing.T)) {
	os.RemoveAll(homePath)
	err := os.Mkdir(homePath, os.FileMode(0755))
	if err != nil {
		t.Error(err)
	}
	err = util.CopyDirectory(dataPath, homePath)
	if err != nil {
		t.Error(err)
	}

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

	return util.Must(loader.LoadOriginals(homePath)),
		func(t *testing.T) {
			os.RemoveAll(homePath)
		}
}

func TestImageHandler(t *testing.T) {
	files, teardown := setupTest(t)
	defer teardown(t)

	// given
	handler := handler.ImageHandler{
		FileEntries: files,
	}
	req := httptest.NewRequest("GET", "http://mock", nil)
	req.SetPathValue("id", "ambience.jpg")
	w := httptest.NewRecorder()

	// when
	handler.Images(w, req)

	// then
	resp := w.Result()
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "image/jpeg", resp.Header.Get("Content-Type"))

	// TODO test size of image
}

func TestImageHandlerNotFound(t *testing.T) {
	files, teardown := setupTest(t)
	defer teardown(t)

	// given
	handler := handler.ImageHandler{
		FileEntries: files,
	}
	req := httptest.NewRequest("GET", "http://mock", nil)
	req.SetPathValue("id", "mock.jpg")
	w := httptest.NewRecorder()

	// when
	handler.Images(w, req)

	// then
	resp := w.Result()
	assert.Equal(t, 404, resp.StatusCode)
}

func TestPreviewHandler(t *testing.T) {
	files, teardown := setupTest(t)
	defer teardown(t)

	// given
	handler := handler.ImageHandler{
		FileEntries: files,
	}
	req := httptest.NewRequest("GET", "http://mock", nil)
	req.SetPathValue("id", "fire.jpg")
	w := httptest.NewRecorder()

	// when
	handler.Previews(w, req)

	// then
	resp := w.Result()
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "image/jpeg", resp.Header.Get("Content-Type"))

	// TODO test size of image
}

func TestPreviewHandlerNotFound(t *testing.T) {
	files, teardown := setupTest(t)
	defer teardown(t)

	// given
	handler := handler.ImageHandler{
		FileEntries: files,
	}
	req := httptest.NewRequest("GET", "http://mock", nil)
	req.SetPathValue("id", "mock-preview.jpg")
	w := httptest.NewRecorder()

	// when
	handler.Previews(w, req)

	// then
	resp := w.Result()
	assert.Equal(t, 404, resp.StatusCode)
}
