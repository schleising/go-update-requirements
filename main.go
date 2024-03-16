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

	// Check if the file exists
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		// Print that file does not exist and exit include the filename in the message
		fmt.Printf("File '%s' does not exist\n", filename)
		os.Exit(1)
	}

	updater.Update(filename)
}
