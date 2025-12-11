package main

import (
	"github.com/dozerokz/logger"
	"github.com/joho/godotenv"
	"os"
	filesreaders "polymarket_monitor/internal/files_readers"
	"polymarket_monitor/internal/notifier"
	"polymarket_monitor/internal/parser"
)

var log *logger.Logger
var wallets []string

func init() {
	log = logger.NewLogger(logger.INFO, logger.INFO)
	err := godotenv.Load()
	if err != nil {
		log.Error("Error loading .env file: %v", err)
		os.Exit(1)
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

	if os.Getenv("TG_BOT_TOKEN") == "" || os.Getenv("CHAT_ID") == "" {
		log.Error("TG_BOT_TOKEN or CHAT_ID in .env is empty")
		os.Exit(1)
	}
}

func main() {
	tgNotifier := notifier.NewTgNotifier(os.Getenv("TG_BOT_TOKEN"), os.Getenv("CHAT_ID"))

	parser.Monitor(wallets, tgNotifier, log)
}
