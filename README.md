# slackflashback
Slack bot written in Go to archive channels and provide keyword search of past messages

Entire channel history is retrieved through the Slack REST and WebSocket API. Messages are stored in a local sqlite3 database, and indexed with FTS to allow for fast keyword search of message texts.

## Building
The mattn/go-sqlite3 dependency makes slackflashback a cgo application, so `gcc` is required to compile. Builds must include the `fts5` tag (i.e. `--tags "fts5"`) to enable sqlite's Full Text Search extension.

## Running
`slackflashback --authtoken=<token> --botname=<name>`

`<token>` is the Slack API token for the bot, and `<name>` is the bot display name.
