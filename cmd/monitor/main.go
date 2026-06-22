package main

import (
	"fmt"
	"github.com/dozerokz/logger"
	"github.com/joho/godotenv"
	"os"
	"polymarket_monitor/internal/activitycache"
	"polymarket_monitor/internal/config"
	filesreaders "polymarket_monitor/internal/files_readers"
	"polymarket_monitor/internal/notifier"
	"polymarket_monitor/internal/parser"
	"strconv"
	"strings"
)

var log *logger.Logger
var wallets []string
var telegramTopicID *int64
var appConfig config.Config

const activityCachePath = "data/activity_cache.sqlite"
const legacyActivityCachePath = "activity_cache.sqlite"

func getOptionalTelegramTopicID() (*int64, error) {
	rawTopicID := strings.TrimSpace(os.Getenv("TG_TOPIC_ID"))
	if rawTopicID == "" {
		return nil, nil
	}

	topicID, err := strconv.ParseInt(rawTopicID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("TG_TOPIC_ID must be an integer: %w", err)
	}

	return &topicID, nil
}

func init() {
	log = logger.NewLogger(logger.INFO, logger.INFO)
	err := godotenv.Load()
	if err != nil {
		log.Error("Error loading .env file: %v", err)
		os.Exit(1)
	}
	if err = os.MkdirAll("logger", 0o755); err != nil {
		log.Error("error while creating logger directory: %v", err)
	}
	err = log.SetLogFile("logger/out.log")
	if err != nil {
		err = log.SetLogFile("out.log")
		if err != nil {
			log.Error("error while creating out.log file: %v", err)
			os.Exit(1)
		}
	}

	wallets, err = filesreaders.ReadTXT("wallets.txt")
	if err != nil {
		log.Error("Error reading wallets.txt file: %v", err)
		os.Exit(1)
	}

	appConfig, err = config.Load("config.yml")
	if err != nil {
		log.Error("%v", err)
		os.Exit(1)
	}

	if os.Getenv("TG_BOT_TOKEN") == "" || os.Getenv("CHAT_ID") == "" {
		log.Error("TG_BOT_TOKEN or CHAT_ID in .env is empty")
		os.Exit(1)
	}

	telegramTopicID, err = getOptionalTelegramTopicID()
	if err != nil {
		log.Error("%v", err)
		os.Exit(1)
	}
}

func main() {
	tgNotifier := notifier.NewTgNotifier(os.Getenv("TG_BOT_TOKEN"), os.Getenv("CHAT_ID"), telegramTopicID)
	parser.SetBlacklistedEventTags(appConfig.NegativeTags)
	if err := activitycache.MigrateLegacyDatabase(legacyActivityCachePath, activityCachePath); err != nil {
		log.Error("Failed to migrate legacy activity cache: %v", err)
		os.Exit(1)
	}
	activityStore, err := activitycache.Open(activityCachePath)
	if err != nil {
		log.Error("Failed to open activity cache: %v", err)
		os.Exit(1)
	}
	defer activityStore.Close()

	parser.Monitor(wallets, tgNotifier, activityStore, log)
}
