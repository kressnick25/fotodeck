package images

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Loader struct {
	OptimisedExtension string
	PreviewExtension   string
}

func (l *Loader) LoadOriginals(homePath string) (map[string]ImageFile, error) {
	if len(l.OptimisedExtension) == 0 || len(l.PreviewExtension) == 0 {
		slog.Error("Optimised or Preview extension is empty", "optExt", l.OptimisedExtension, "prvExt", l.PreviewExtension)
		return nil, fmt.Errorf("Optimised or Preview extension is empty optimised='%s', preview='%s'", l.OptimisedExtension, l.PreviewExtension)
	}

	slog.Info("Loading original Images from homePath", "path", homePath, "class", "Loader")
	fileMap := make(map[string]ImageFile)
	err := filepath.WalkDir(homePath, func(path string, f os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !f.Type().IsRegular() {
			return nil
		}
		if strings.Contains(f.Name(), l.OptimisedExtension) || strings.Contains(f.Name(), l.PreviewExtension) {
			slog.Debug("skipping already optimised file", "path", f.Name(), "class", "Loader", "optExt", l.OptimisedExtension, "prvExt", l.PreviewExtension)
			return nil
		}
		if !isFiletypeAllowed(f.Name()) {
			slog.Debug("skipping non-image file", "path", f.Name(), "class", "Loader")
			return nil
		}

		existingPath, ok := fileMap[f.Name()]
		if ok {
			slog.Warn("duplicate filename entry found (existingPath). path will be used instead", "path", path, "existingPath", existingPath)
		}

		fileMap[f.Name()] = NewImageFile(f.Name(), path)

		return nil
	})
	if err != nil {
		return nil, err
	}

	return fileMap, nil
}

func (l *Loader) IsResizedImage(path string) bool {
	return strings.Contains(path, "."+l.OptimisedExtension+".") || strings.Contains(path, "."+l.PreviewExtension+".")
}

func (l *Loader) worker(maxOD Dimensions, maxPd Dimensions, optExt string, prvExt string, jobs <-chan struct {
	string
	ImageFile
}, results chan<- struct {
	string
	ImageFile
}) {
	for item := range jobs {
		key := item.string
		image := item.ImageFile

		optimised, err := l.OptimiseImage(image, maxOD, maxPd, optExt, prvExt)
		if err != nil {
			slog.Error("optimiseImageError", "error", err)
		}

		results <- struct {
			string
			ImageFile
		}{key, optimised}
	}
}

func (l *Loader) OptimiseImages(images *map[string]ImageFile, maxOptimisedDimensions Dimensions, maxPreviewDimensions Dimensions) error {
	numCpus := runtime.NumCPU()
	imageCount := len(*images)

	// anonymous struct to hold the key, value pairs
	results := make(chan struct {
		string
		ImageFile
	}, imageCount)

	jobs := make(chan struct {
		string
		ImageFile
	}, imageCount)

	for i := 0; i < numCpus; i++ {
		go l.worker(maxOptimisedDimensions, maxPreviewDimensions, l.OptimisedExtension, l.PreviewExtension, jobs, results)
	}

	for k, v := range *images {
		jobs <- struct {
			string
			ImageFile
		}{k, v}
	}
	close(jobs)

	for i := 0; i < imageCount; i++ {
		item := <-results
		key := item.string
		val := item.ImageFile

		(*images)[key] = val
	}
	close(results)

	return nil
}

func (l *Loader) OptimiseImage(image ImageFile, maxOptimisedDimensions Dimensions, maxPreviewDimensions Dimensions, optimisedExt string, previewExt string) (ImageFile, error) {
	optimisedPath := l.resizeImage(image.GetFullSize(), optimisedExt, maxOptimisedDimensions.Width, maxOptimisedDimensions.Height)
	previewPath := l.resizeImage(image.GetFullSize(), previewExt, maxPreviewDimensions.Width, maxPreviewDimensions.Height)

	return ImageFile{
		name:          image.Name(),
		originalPath:  image.GetFullSize(),
		optimisedPath: optimisedPath,
		previewPath:   previewPath,
	}, nil
}

func (l *Loader) resizeImage(inputPath string, extension string, maxWidth int, maxHeight int) string {
	outputPath := l.getOptimisedFilePath(inputPath, extension)
	if _, err := os.Stat(outputPath); err == nil {
		slog.Debug("Resized image already exists, skipping", "path", filepath.Clean(outputPath))
		return outputPath
	}

	slog.Info("resizing image", "extension", extension, "path", filepath.Clean(outputPath))
	image, err := Open(inputPath)
	if err != nil {
		slog.Error("error opening image to resize", "error", err)
		return inputPath
	}

	opts := ResizeOptions{
		MaxWidth:  maxWidth,
		MaxHeight: maxHeight,
	}
	image = Resize(image, opts)

	err = Save(image, outputPath)
	if err != nil {
		slog.Error("error saving image to resize", "error", err)
		return inputPath
	}

	return outputPath
}

func (l *Loader) getOptimisedFilePath(inputPath string, extension string) string {
	paths := strings.Split(inputPath, ".")

	if l.IsResizedImage(inputPath) {
		// already an optimised file
		return inputPath
	}

	// transform 'image.jpg' -> 'image.optimised.jpg'
	tmp := paths[len(paths)-1]
	paths[len(paths)-1] = extension
	paths = append(paths, tmp)

	return strings.Join(paths, ".")
}

func isFiletypeAllowed(fileName string) bool {
	whitelist := []string{"png", "jpeg", "jpg", "svg", "gif"}
	_type := fileName[strings.LastIndex(fileName, ".")+1:]

	return stringInSlice(strings.ToLower(_type), whitelist)
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
