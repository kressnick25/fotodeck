package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
			if err != nil {
				fmt.Println("Walkdir error: ", path, err)
			}
			if !strings.Contains(f.Name(), ".opt.") && !strings.Contains(f.Name(), ".prev.") {
				return nil
			}

			fmt.Println("Removing file: ", path)
			err = os.Remove(path)
			if err != nil {
				fmt.Println("Failed to remove file: ", path, err)
			}

			return nil
		})
		if err != nil {
			fmt.Println("Error cleaning up homePath: ", err)
			os.Exit(1)
		}
	}
}
