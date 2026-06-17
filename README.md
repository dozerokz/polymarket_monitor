# Polymarket Monitor

A lightweight Go application that monitors Polymarket wallet activity and sends real-time notifications to Telegram when trades are detected.

![Polymarket monitor](https://i.imgur.com/PkGbdo8.png)


## Features

- Monitors multiple Polymarket wallet addresses simultaneously

- Tracks buy and sell trades in real-time

- Sends formatted notifications to Telegram

## Prerequisites

- Go 1.19 or higher

- Telegram Bot Token

- Telegram Chat ID

## Installation

1. Clone the repository:

```bash
git clone https://github.com/dozerokz/polymarket_monitor.git
cd polymarket_monitor
```

2. Install dependencies:

```bash
go mod download
```
3. Create a `.env` file in the root directory:

```bash
cp .env.example .env
```

Then edit `.env`:

```env
TG_BOT_TOKEN=your_telegram_bot_token_here
CHAT_ID=your_telegram_chat_id_here
TG_TOPIC_ID=your_optional_telegram_topic_id_here
```

4. Edit `config.yml` if you want to change negative tag filters:

```yaml
negative_tags:
  - sports
  - esports
```

5. Paste POLYMARKET wallet addresses into `wallets.txt` file in the root directory(one per line)

## Configuration

The .env file requires the following variables:


- `TG_BOT_TOKEN` - Your Telegram bot token (obtain from @BotFather)

- `CHAT_ID` - The Telegram chat ID where notifications will be sent

- `TG_TOPIC_ID` - Optional Telegram forum topic ID; if empty, messages are sent to the main chat

- `config.yml` - YAML config for app-level settings such as `negative_tags`

The `.env` file is not included in the repository for security reasons.
Use `.env.example` as a template.

## Usage

Run the application:

```bash
go run cmd/monitor/main.go
```

Or build and run the executable:

```bash
go build -o polymarket_monitor cmd/monitor/main.go
./polymarket_monitor
```

## Project Structure
```
polymarket_monitor/
├── cmd/
│   └── monitor/
│       └── main.go
├── internal/
│   ├── config/
│   │   ├── config.go
│   │   └── config_test.go
│   ├── files_readers/
│   │   └── files_readers.go
│   ├── notifier/
│   │   └── telegram.go
│   └── parser/
│       ├── parser.go
│       └── model.go
├── config.yml
├── .env (create this)
├── .env.example
├── wallets.txt (edit this)
├── go.mod
├── go.sum
├── LICENSE
└── README.md
```

## How It Works

- Reads wallet addresses from wallets.txt

- Initializes monitoring by fetching recent activity for each wallet

- Continuously polls the Polymarket API for new activity

- Compares new activity against cached data

- Skips Telegram notifications for events with tags listed in `config.yml`

- Sends Telegram notifications for detected trades

- Updates cache with latest activity

## Notes

- Poll interval is 5 seconds; on Polymarket API errors a wallet is retried after 15 seconds.
- Logs are written to `logger/out.log`.

## License

This project is open-source. You can use, modify, and distribute it under the [MIT License](LICENSE).
## Contributing

Contributions are welcome. Please open an issue or submit a pull request.
