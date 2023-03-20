package main

import (
	handler "app/handler"
	"flag"
	"log"
	"os"
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// LogLevel is the level of logging
type LogLevel int

const (
	INFO LogLevel = iota
	DEBUG
)

var (
	infoLogger  *log.Logger
	debugLogger *log.Logger
)

// appConfig contains app info
type appConfig struct {
	AppName      string `yaml:"appName"`
	Port         int    `yaml:"port"`
	PostTempURL  string `yaml:"postTempURL"`
	GetErrorsURL string `yaml:"getErrorsURL"`
	ReadmeURL    string `yaml:"readmeURL"`
	DeleteURL    string `yaml:"deleteURL"`
}

// Obfuscate obfuscates the config
func (config *appConfig) GetAppName() string {
	return config.AppName
}

// Obfuscate obfuscates the config
func (config *appConfig) Validate() error {
	if config.Port <= 0 {
		return errors.Errorf("invalid port: %d for HTTP server", config.Port)
	}
	return nil
}

// parseConfig parses the config file and returns the config object
func ParseConfig(config interface{}, args []string) error {
	flagSet := flag.NewFlagSet(args[0], flag.ContinueOnError)
	configPathStr := flagSet.String("config", "", "configuration files")
	if err := flagSet.Parse(args[1:]); err != nil {
		return err
	}
	commaSeparatedPaths := *configPathStr
	configPaths := strings.Split(commaSeparatedPaths, ",")

	var configByte []byte
	if len(configPaths) == 1 {
		configBytes, err := os.ReadFile(configPaths[0])
		if err != nil {
			return errors.Errorf("error read file. path: %s, error: %v", configPaths[0], err)
		}

		configByte = configBytes
	}
	err := yaml.Unmarshal(configByte, config)
	if err != nil {
		return errors.Errorf("failed to unmarshal config. configPath: %s, error: %v", *configPathStr, err)
	}

	return nil
}

// runMain is the main function
func runMain(args []string) {
	config := &appConfig{}

	logger := log.New(os.Stdout, "DEBUG: ", log.Ldate|log.Ltime|log.Lshortfile)

	if err := ParseConfig(config, args); err != nil {
		logger.Fatalf("ParseConfig failed. error: %v", err)
	}

	logger.Printf("Starting %s", config.AppName)
	router := handler.NewRouter()
	router.GET(config.ReadmeURL, handler.GetReadmeHandler(logger))
	router.GET(config.GetErrorsURL, handler.GetErrorsHandler(logger))
	router.DELETE(config.DeleteURL, handler.DeleteHandler(logger))
	router.POST(config.PostTempURL, handler.PostTempHandler(logger))

	server := handler.NewServer(config.Port, router)

	logger.Printf(
		"HTTP server running at Port: %d. GetErrors URL: %s, PostTemp URL: %s, "+
			"Delete URL: %s",
		config.Port, config.GetErrorsURL, config.PostTempURL,
		config.DeleteURL)

	if err := server.ListenAndServe(); err != nil {
		log.Panicf("Failed to start HTTP server. Reason: %v", err)
	}

	logger.Printf("%s exiting", config.AppName)
}

func main() {
	runMain(os.Args)
}
