package images

import (
	"log"
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
	log.Println("Loading homePath from: ", homePath)
	fileMap := make(map[string]ImageFile)
	err := filepath.WalkDir(homePath, func(path string, f os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !f.Type().IsRegular() {
			return nil
		}
		if strings.Contains(f.Name(), l.OptimisedExtension) || strings.Contains(f.Name(), l.PreviewExtension) {
			// TODO debug log
			log.Println("loader: skipping already optimised file: ", f.Name())
			return nil
		}
		if !isFiletypeAllowed(f.Name()) {
			// TODO debug log
			log.Println("loader: skipping non-image file: ", f.Name())
			return nil
		}

		existingPath, ok := fileMap[f.Name()]
		if ok {
			log.Printf("loader: warning: duplicate filename entry found at %s. %s will be used\n", existingPath, path)
		}

		fileMap[f.Name()] = NewImageFile(f.Name(), path)

		return nil
	})
	if err != nil {
		return nil, err
	}

	return fileMap, nil
}

func worker(maxOD Dimensions, maxPd Dimensions, jobs <-chan struct {
	string
	ImageFile
}, results chan<- struct {
	string
	ImageFile
}) {
	for item := range jobs {
		key := item.string
		image := item.ImageFile

		optimised, err := OptimiseImage(image, maxOD, maxPd)
		if err != nil {
			// TODO error log
			log.Println("optimiseImageError: ", err)
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
		go worker(maxOptimisedDimensions, maxPreviewDimensions, jobs, results)
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

func OptimiseImage(image ImageFile, maxOptimisedDimensions Dimensions, maxPreviewDimensions Dimensions) (ImageFile, error) {
	optimisedPath := resizeImage(image.GetFullSize(), OptimisedExtension, maxOptimisedDimensions.Width, maxOptimisedDimensions.Height)
	previewPath := resizeImage(image.GetFullSize(), PreviewExtension, maxOptimisedDimensions.Width, maxOptimisedDimensions.Height)

	image.SetOptimisedPath(optimisedPath)
	image.SetPreviewPath(previewPath)

	return ImageFile{
		name:          image.Name(),
		originalPath:  image.GetFullSize(),
		optimisedPath: optimisedPath,
		previewPath:   previewPath,
	}, nil
}

func resizeImage(inputPath string, extension string, maxWidth int, maxHeight int) string {
	outputPath := getOptimisedFilePath(inputPath, extension)
	if _, err := os.Stat(outputPath); err == nil {
		// TODO debug log
		log.Println("resizeImage: Resized image already exists, skipping: ", filepath.Clean(outputPath))
		return outputPath
	}

	log.Printf("resizeImage: generating %s image: %s\n", extension, filepath.Clean(outputPath))
	image, err := Open(inputPath)
	if err != nil {
		log.Println("resizeImage: Error opening image to resize: ", err)
		return inputPath
	}

	opts := ResizeOptions{
		MaxWidth:  maxWidth,
		MaxHeight: maxHeight,
	}
	image = Resize(image, opts)

	err = Save(image, outputPath)
	if err != nil {
		log.Println("resizeImage: error saving image to resize: ", err)
		return inputPath
	}

	return outputPath
}

func getOptimisedFilePath(inputPath string, extension string) string {
	paths := strings.Split(inputPath, ".")

	if IsResizedImage(inputPath) {
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
