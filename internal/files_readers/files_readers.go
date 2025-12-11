package filesreaders

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// ReadTXT reads lines from .txt file to slice of strings
func ReadTXT(filePath string) ([]string, error) {
	exeDir, err := getExecutableDir()
	if err != nil {
		return nil, err
	}

	file, err := os.Open(filepath.Join(exeDir, filePath))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}

	if err = scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}

// getExecutableDir determines the correct directory based on how the program is running
func getExecutableDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}

	// Check if running with "go run"
	if strings.Contains(exePath, "go-build") || strings.Contains(exePath, "tmp") {
		// Running with go run, use current working directory
		var wd string
		wd, err = os.Getwd()
		if err != nil {
			return "", err
		}
		return wd, nil
	}

	// Running as compiled executable, use executable directory
	return filepath.Dir(exePath), nil
}
