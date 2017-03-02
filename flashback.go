package main

import (
	"fmt"
	"github.com/nlopes/slack"
)

func main() {
	slackApi := slack.New(getSlackToken())
	groups, err := slackApi.GetGroups(false)

	if err != nil {
		fmt.Printf("%s\n", err)
		return
	}

	for _, group := range groups {
		fmt.Printf("ID: %s, Name: %s\n", group.ID, group.Name)
	}
}

func getSlackToken() string {
	return "xoxb-114019759923-fSW5MUF2u8XECr9Ajt8YQ4p1"
}
