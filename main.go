package main

import (
	"github.com/fatih/color"
	"os"
	"schleising.net/updater"
)

func main() {
	// Declare a variable to hold the filenames
	var filenames []string;
	
	// Read a filename from the command line
	if len(os.Args) < 2 {
		// Declare a variable to hold the error
		var err error;

		// Find all files called requirements.txt recursively from the current directory
		filenames, err = updater.FindRequirements()

		// Check if there was an error
		if err != nil {
			color.Red("Error: %s", err)
			os.Exit(1)
		}
	} else {
		// Get the filenames from the command line
		filenames = os.Args[1:]
	}

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
			color.Green("%s updated successfully", filename)
		}
	}
}
