package main

import (
	"fmt"
	"time"
	"strconv"
	"github.com/nlopes/slack"
)

var botId string

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

	for _, user := range users {
		if user.Name == getBotName() {
			botId = user.ID
			fmt.Printf("Bot id: %s\n", botId)
		}
	}

	if botId == "" {
		fmt.Println("Bot id not found")
		return
	}

	rtm := slackApi.NewRTM()

	go rtm.ManageConnection()

	for msg := range rtm.IncomingEvents {
		switch msgData := msg.Data.(type) {
		case *slack.MessageEvent:
			timestamp, _ := strconv.ParseFloat(msgData.Timestamp, 32)
			fmt.Printf("%s [user:%s|channel:%s] %s\n", time.Unix(int64(timestamp), 0), msgData.User, msgData.Channel, msgData.Msg.Text)
			break
		}
	}
}

func getSlackToken() string {
	return "xoxb-114019759923-fSW5MUF2u8XECr9Ajt8YQ4p1"
}

func getBotName() string {
	return "inetcobot"
}