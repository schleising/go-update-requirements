package main

import (
	"github.com/fatih/color"
	"os"
	"schleising.net/updater"
)

func main() {
	// Read a filename from the command line
	if len(os.Args) < 2 {
		color.Red("Usage: %s <filename> [<filename> ...]", os.Args[0])
		os.Exit(1)
	}

	// Get the filenames from the command line
	filenames := os.Args[1:]

	// Loop through the filenames
	for _, filename := range filenames {
		// Call the Update function from the updater package
		if err := updater.UpdateRequirements(filename); err != nil {
			if os.IsNotExist(err) {
				color.Red("Error: File %s does not exist", filename)
			} else {
				color.Red("Error: %s", err)
			}
			os.Exit(1)
		} else {
			color.Green("Requirements updated successfully")
		}
	}
}
