package main

import (
	"flag"
	"github.com/fatih/color"
	"os"
	"schleising.net/updater"
)

const application_version = "1.1.6"

func main() {
	// Add a flag to print the version
	version := flag.Bool("v", false, "Print the version and exit")

	// Set the usage message
	flag.Usage = func() {
		color.Cyan("Requirement Updater Version: %s", application_version)
		println()
		color.Cyan("Usage: %s [filename...]", os.Args[0])
		flag.PrintDefaults()
	}

	// Parse the command line flags
	flag.Parse()

	// Check if the version flag was set
	if *version {
		color.Cyan("Requirement Updater Version %v", application_version)
		os.Exit(0)
	}

	// Print the version
	color.Cyan("Requirement Updater Version %v", application_version)

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
			boldGreen := color.New(color.FgGreen, color.Bold)
			boldGreen.Printf("%s updated successfully\n", filename)
		}
	}
}
