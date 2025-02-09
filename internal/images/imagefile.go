package images

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const OptimisedExtension = "optimised"
const PreviewExtension = "preview"

func IsResizedImage(path string) bool {
	return strings.Contains(path, "."+OptimisedExtension+".") || strings.Contains(path, "."+PreviewExtension+".")
}

type ImageFile struct {
	originalPath  string
	optimisedPath string
	previewPath   string
	name          string
}

func NewImageFile(name string, path string) ImageFile {
	return ImageFile{
		name:          name,
		originalPath:  path,
		optimisedPath: "",
		previewPath:   "",
	}
}

func (i *ImageFile) GetPreview() string {
	if i.previewPath == "" {
		return i.originalPath
	}
	return i.previewPath
}

func (i *ImageFile) GetFullSize() string {
	if i.optimisedPath == "" {
		return i.originalPath
	}
	return i.optimisedPath
}

func (i *ImageFile) Name() string {
	return i.name
}

func (i *ImageFile) IsOptimised() bool {
	return i.optimisedPath != ""
}

func (i *ImageFile) Cleanup() error {
	if i.optimisedPath != "" {
		slog.Info("removing optimised file", "path", filepath.Clean(i.optimisedPath))
		err := os.Remove(i.optimisedPath)
		if err != nil {
			return err
		}
	}
	if i.previewPath != "" {
		slog.Info("removing preview file", "path", filepath.Clean(i.optimisedPath))
		err := os.Remove(i.previewPath)
		if err != nil {
			return err
		}
	}
	return nil
}
