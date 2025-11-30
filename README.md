# GGP - Golden Gym Predictor Telegram Bot

A Telegram bot application with SQLite database support.

## Features

- ...

## Requirements

- Go 1.25 or later
- Telegram Bot API token (obtain from [@BotFather](https://t.me/botfather))

## Installation

```bash
go build -o ggp .
```

## Configuration

Create a configuration file based on the example:

```bash
cp config.example.toml config.toml
```

Edit `config.toml` with your settings:

```toml
[telegram]
token = "YOUR_TELEGRAM_BOT_TOKEN"

[database]
path = "ggp.db"
```

## Usage

```bash
./ggp -config config.toml
```

## Project Structure

```
.
├── config/          # Configuration package
├── databaser/       # SQLite database package
├── main.go          # Application entry point
├── config.example.toml  # Example configuration
└── README.md
```

## Dependencies

- [go-telegram/bot](https://github.com/go-telegram/bot) - Telegram Bot API
- [pelletier/go-toml/v2](https://github.com/pelletier/go-toml) - TOML configuration
- [modernc.org/sqlite](https://modernc.org/sqlite) - SQLite driver (pure Go)
- [jmoiron/sqlx](https://github.com/jmoiron/sqlx) - Extensions to database/sql

## License

MIT License - see [LICENSE](LICENSE) file for details.
