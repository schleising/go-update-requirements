package updater

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"github.com/fatih/color"
)

func RemoveVersions(filename string) error {
	// Check if the file exists and is a regular file
	if info, err := os.Stat(filename); err != nil {
		return err
	} else if info.IsDir() {
		return fmt.Errorf("'%s' is a directory", filename)
	} else if !info.Mode().IsRegular() {
		return fmt.Errorf("'%s' is not a regular file", filename)
	}

	// Print a message to the console to indicate that the file is being read
	color.Blue("Reading '%s'", filename)

	// Open the file for reading
	if file, err := os.OpenFile(filename, os.O_RDWR, 0644); err != nil {
		return err
	} else {
		// Defer closing the file
		defer file.Close()

		// Create a list of strings to hold the file contents
		var lines []string

		// Read the file line by line
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())

			// Check for errors
			if scanner.Err() != nil {
				return scanner.Err()
			}
		}

		// Print a message to the console to indicate that the requirement versions are being removed
		color.Blue("Removing requirement versions")

		// Create a list of strings to hold the updated file contents
		var updated_lines []string

		// Remove all text after and including the first == occurrence
		separator := "=="
		for _, line := range lines {
			before, _, is_cut := strings.Cut(line, separator)

			if is_cut {
				updated_lines = append(updated_lines, before)
			} else {
				updated_lines = append(updated_lines, line)
			}
		}

		// Print a message to the console to indicate that the updated file contents are being written
		color.Blue("Writing updated file contents")

		// Truncate the file
		if err := file.Truncate(0); err != nil {
			return err
		}

		// Seek to the beginning of the file
		if _, err := file.Seek(0, 0); err != nil {
			return err
		}

		// Write the updated file contents to the file
		for _, line := range updated_lines {
			_, err = file.WriteString(line + "\n")

			// Check for errors
			if err != nil {
				return err
			}
		}
	}

	// Return success
	return nil
}
