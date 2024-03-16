package main

import (
	"os"
	"schleising.net/updater"
	"github.com/fatih/color"
)

func main() {
	// Read a filename from the command line
	if len(os.Args) < 2 {
		color.Red("Usage: updater <filename>")
		os.Exit(1)
	}

	// Get the filename from the command line
	filename := os.Args[1]

	// Call the Update function from the updater package
	if err := updater.RemoveVersions(filename); err != nil {
		if os.IsNotExist(err) {
			color.Red("Error: File '%s' does not exist\n", filename)
		} else {
			color.Red("Error: %s\n", err)
		}
		os.Exit(1)
	} else {
		color.Green("File updated successfully")
	}
}
