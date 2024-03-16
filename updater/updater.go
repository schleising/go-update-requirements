package updater

import (
	"fmt"
	"os"
)

func Update(filename string) (error) {
	// Check if the file exists
	_, err := os.Stat(filename)
	
	// If the file does not exist, return an error
	if err != nil {
		return err
	}

	// Print a message
	fmt.Printf("In Update with file '%s'\n", filename)

	// If the file exists, return nil
	return nil
}
