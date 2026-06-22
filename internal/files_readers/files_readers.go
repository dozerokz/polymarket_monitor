package filesreaders

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ReadTXT reads lines from .txt file to slice of strings
func ReadTXT(filePath string) ([]string, error) {
	resolvedPath, err := ResolvePath(filePath)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(resolvedPath)
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

// ReadYAML reads YAML file content into out.
func ReadYAML(filePath string, out any) error {
	resolvedPath, err := ResolvePath(filePath)
	if err != nil {
		return err
	}

	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		return err
	}

	return yaml.Unmarshal(content, out)
}

// ResolvePath resolves file paths relative to the executable/current working directory.
func ResolvePath(filePath string) (string, error) {
	if filepath.IsAbs(filePath) {
		return filePath, nil
	}

	exeDir, err := getExecutableDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(exeDir, filePath), nil
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
