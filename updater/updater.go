package updater

import (
	"bufio"
	"fmt"
	"github.com/fatih/color"
	"os"
	"os/exec"
	"strings"
)

func UpdateRequirements(filename string) error {
	// Print a message to the console to indicate that the file is being processed
	fmt.Println()
	color.Blue("Processing %s", filename)

	// Check if the file exists and is a regular file
	if info, err := os.Stat(filename); err != nil {
		return err
	} else if info.IsDir() {
		return fmt.Errorf("%s is a directory", filename)
	} else if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", filename)
	} else if info.Mode().Perm() & 0200 == 0 {
		return fmt.Errorf("%s is not writable", filename)
	} else if info.Mode().Perm() & 0400 == 0 {
		return fmt.Errorf("%s is not readable", filename)
	}

	// Call the removeVersions function
	if err := removeVersions(filename); err != nil {
		return err
	}

	// Call the uninstallPackages function
	if err := uninstallPackages(); err != nil {
		return err
	}	

	// Call the installPackages function
	if err := installPackages(filename); err != nil {
		return err
	}

	// Return success
	return nil
}

func removeVersions(filename string) error {
	// Print a message to the console to indicate that the file is being read
	color.Blue("Updating %s", filename)

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

	// Print a message to the console to indicate that the file has been updated
	color.Green("%s updated successfully", filename)

	// Return success
	return nil
}

// Function to use pip to uninstall the current list of packages
func uninstallPackages() error {
	// Print a message to the console to indicate that the packages are being uninstalled
	color.Blue("Uninstalling packages")

	// Create a new command to list the installed packages
	cmd := exec.Command("pip", "freeze")

	// Create a list of strings to hold the installed packages
	var packageList []string

	// Redirect the standard output to an array of strings
	if output, err := cmd.Output(); err != nil {
		return err
	} else {
		// Convert the output to a string
		string_output := string(output)

		// Split the output into an array of strings
		packageList = strings.Split(string_output, "\n")
	}

	// Separator for packages installed from Git
	separator := " @ "

	// Create a list of strings to hold the packages to uninstall
	var args []string

	// Add the uninstall command and the -y flag to the list of packages to uninstall
	args = append(args, "uninstall")
	args = append(args, "-y")

	// Loop through the packages
	for _, pkg := range packageList {
		// Check if the package was installed from Git
		pkg, _, _ := strings.Cut(pkg, separator)

		// Add the package to the list of packages to uninstall
		if pkg != "" {
			args = append(args, pkg)
		}
	}

	// Check if there are any packages to uninstall
	if len(args) > 2 {
		// Create a new command to uninstall the package
		cmd = exec.Command("pip", args...)

		// Run the command
		if err := cmd.Run(); err != nil {
			return err
		}
	}

	// Print a message to the console to indicate that the packages have been uninstalled
	color.Green("Packages uninstalled successfully")

	// Return success
	return nil
}

// Function to use pip to install the latest version of the packages
func installPackages(filename string) error {
	// Print a message to the console to indicate that the packages are being installed
	color.Blue("Installing packages")

	// Create a new command to install the latest version of the packages
	cmd := exec.Command("pip", "install", "-r", filename)

	// Run the command
	if err := cmd.Run(); err != nil {
		return err
	}

	// Print a message to the console to indicate that the packages have been installed
	color.Green("Packages installed successfully")

	// Update the list of packages to the provided file
	cmd = exec.Command("pip", "freeze")

	// Redirect the standard output to the provided file
	if file, err := os.Create(filename); err != nil {
		return err
	} else {
		cmd.Stdout = file
		defer file.Close()
	}

	// Run the command
	if err := cmd.Run(); err != nil {
		return err
	}

	// Return success
	return nil
}
