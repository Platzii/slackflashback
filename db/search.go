package db

import (
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"regexp"
	"strings"
)

type command string

type Search struct {
	userMap       map[string]string
	botId         string
	botName       string
	commandRegExp *regexp.Regexp
}

var keywordRegExp *regexp.Regexp = regexp.MustCompile(`\w+`)

// Sets bot information.
func (s *Search) SetBotInfo(botId string, botName string) (err error) {
	s.botId = botId
	s.botName = botName
	// Build the message match pattern to filter for messages mentioning the bot's name
	if s.commandRegExp, err = regexp.Compile(`(^|\W+)<@` + botId + `>:\W*(.+)`); err != nil {
		return err
	}
	return nil
}

// Sets the user-id-to-user-name mapping.
// This mapping is used to translate user names to ids for use in queries.
func (s *Search) SetUserMap(userMap map[string]string) {
	s.userMap = userMap
}

// Processes a command message and produces a database-friendly query.
func (s *Search) GetQueryFromCommand(cmd string) (string, error) {
	match := s.commandRegExp.FindStringSubmatch(cmd)
	if match == nil || len(match) < 3 {
		return "", errors.New(fmt.Sprintf("Invalid command: %q", cmd))
	}
	cmdSubStr := match[2]
	keywords := keywordRegExp.FindAllString(cmdSubStr, -1)
	query := strings.Join(keywords, " + ")
	log.Debugf("Query: %s", query)
	return query, nil
}

// Checks if a message is a command for the bot.
// Returns true if this message is a bot command, or false otherwise.
func (s *Search) IsCommand(command string) bool {
	return s.commandRegExp.MatchString(command)
}
