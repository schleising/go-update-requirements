package main

import (
	"fmt"
	"os"
	"schleising.net/updater"
)

func main() {
	fmt.Println("In main")

	// Read a filename from the command line
	if len(os.Args) < 2 {
		fmt.Println("Usage: updater <filename>")
		os.Exit(1)
	}

	// Get the filename from the command line
	filename := os.Args[1]

	// Call the Update function from the updater package
	err := updater.RemoveVersions(filename)

	// Check for errors
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("Error: File '%s' does not exist\n", filename)
		} else {
			fmt.Printf("Error: %s\n", err)
		}
		os.Exit(1)
	} else {
		fmt.Println("File updated successfully")
	}
}
