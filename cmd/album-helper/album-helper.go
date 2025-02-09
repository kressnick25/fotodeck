package main

import (
	"album/internal/images"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Println("USAGE: ./album-helper <COMMAND> <HOME PATH>\nValid commands: [cleanup,]")
		os.Exit(1)
	}
	command := os.Args[1]
	homePath := os.Args[2]

	if command == "cleanup" {
		fmt.Println("Cleaning up image previews for homePath: ", homePath)
		err := filepath.WalkDir(homePath, func(path string, f os.DirEntry, err error) error {
			if !images.IsResizedImage(path) {
				return nil
			}

			fmt.Println("Removing file: ", path)
			os.Remove(path)

			return nil
		})
		if err != nil {
			fmt.Println("Error cleaning up homePath: ", err)
			os.Exit(1)
		}
	}
}
