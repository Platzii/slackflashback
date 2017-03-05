package main

import (
	"fmt"
	"strconv"
	"github.com/nlopes/slack"
	"earthgrazer.ca/slackflashback/db"
	"os"
	"flag"
)

var botId string

func main() {
	parseParams()

	defer db.Close()

	if ready, err := db.IsReady(); !ready {
		fmt.Printf("Database not ready: %q\n", err)
		os.Exit(1)
	}

	slackApi := slack.New(getSlackToken())
	groups, err := slackApi.GetGroups(false)

	if err != nil {
		fmt.Printf("%s\n", err)
		os.Exit(1)
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
		os.Exit(1)
	}

	rtm := slackApi.NewRTM()

	go rtm.ManageConnection()

	// Start processing new messages from all channels that the bot belongs to
	for msg := range rtm.IncomingEvents {
		switch msgData := msg.Data.(type) {
		case *slack.MessageEvent:
			if msgData.User == botId {
				// Ignore any messages from self
				break
			}

			timestamp, _ := strconv.ParseFloat(msgData.Timestamp, 32)
			// Add new message to database
			db.AddMessage(msgData.User, msgData.Channel, int(timestamp), msgData.Msg.Text)
			break
		}
	}
}

func parseParams() {
	authToken = flag.String("authtoken", "", "Slack bot authentication token")
	botName = flag.String("botname", "", "Slack bot name")
	flag.Parse()
}

var (
	authToken *string
	botName *string
)

func getSlackToken() string {
	return *authToken
}

func getBotName() string {
	return *botName
}