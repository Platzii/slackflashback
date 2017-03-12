package main

import (
	"errors"
	log "github.com/sirupsen/logrus"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var userMentionRegExp = regexp.MustCompile(`<@(\w{9})>`)

type userIdSubtitution struct {
	userMapping map[string]string
}

func SubstituteUserIdWithName(userMapping map[string]string, msg string) string {
	sub := userIdSubtitution{userMapping: userMapping}
	return userMentionRegExp.ReplaceAllStringFunc(msg, sub.substituteUserIdWithName)
}

func (u *userIdSubtitution) substituteUserIdWithName(msg string) string {
	userName := ""
	matches := userMentionRegExp.FindStringSubmatch(msg)
	if len(matches) != 2 {
		log.Debug("Invalid user mention format")
		userName = "user"
	} else {
		if name, ok := u.userMapping[matches[1]]; !ok {
			log.Debug("User mention id not found in name mapping")
			userName = "user"
		} else {
			userName = name
		}
	}
	return "@" + userName
}

func ConvertTimestampToString(ts string) (string, error) {
	parts := strings.Split(ts, ".")
	if len(parts) != 2 {
		return "", errors.New("Invalid timestamp format")
	}

	timeInt, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return "", err
	}

	t := time.Unix(timeInt, 0)
	return t.Format(time.UnixDate), nil
}
