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

	fmt.Println("Channels:")

	for _, group := range groups {
		fmt.Printf("ID: %s, Name: %s\n", group.ID, group.Name)
	}

	var users []slack.User

	users, err = slackApi.GetUsers()

	if err != nil {
		fmt.Printf("%s\n", err)
	}

	fmt.Println("Users:")

	for _, user := range users {
		fmt.Printf("ID: %s, Name: %s\n", user.ID, user.Name)
	}
}

func getSlackToken() string {
	return "xoxb-114019759923-fSW5MUF2u8XECr9Ajt8YQ4p1"
}
