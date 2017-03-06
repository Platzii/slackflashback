package main

import (
	"fmt"
	"github.com/nlopes/slack"
	"earthgrazer.ca/slackflashback/db"
	"os"
	"flag"
	"sync"
	"errors"
)

// Configurable parameters
var (
	authToken *string
	botName *string
)

var (
	botId string
	slackApi *slack.Client
)

var (
	userIdNameMap map[string]string = make(map[string]string)
	channelInfoMap channelMap
)

func init() {
	parseParams()
	slackApi = slack.New(getSlackToken())
}

func parseParams() {
	authToken = flag.String("authtoken", "", "Slack bot authentication token")
	botName = flag.String("botname", "", "Slack bot name")
	flag.Parse()
}

func getSlackToken() string {
	return *authToken
}

func getBotName() string {
	return *botName
}

type channelMap struct {
	channels map[string]channel
	sync.Mutex
}

func (m *channelMap) init() {
	m.channels = make(map[string]channel)
}

func (m *channelMap) update() error {
	m.Lock(); defer m.Unlock()

	updatedChannels := []string{}

	fmt.Println("Updating channel information...")

	groups, err := slackApi.GetGroups(true)
	if err != nil {
		return err
	}

	// Add non-existing private channels that this bot belongs to
	for _, group := range groups {
		updatedChannels = append(updatedChannels, group.ID)
		_, exists := m.channels[group.ID]

		if !exists {
			m.channels[group.ID] = channel {
				id: group.ID,
				name: group.Name,
				isPublic: false,
			}
			fmt.Printf("Added private channel [Name=%q,ID=%q]\n", group.Name, group.ID)
		}
	}

	channels, err := slackApi.GetChannels(true)
	if err != nil {
		return err
	}

	// Add non-existing public channels that this bot belongs to
	for _, chann := range channels {
		if !chann.IsMember {
			continue
		}

		updatedChannels = append(updatedChannels, chann.ID)
		_, exists := m.channels[chann.ID]

		if !exists {
			m.channels[chann.ID] = channel {
				id: chann.ID,
				name: chann.Name,
				isPublic: true,
			}
			fmt.Printf("Added public channel [Name=%q,ID=%q]\n", chann.Name, chann.ID)
		}
	}

	// Remove existing channels that this bot no longer belongs to
	for _, group := range m.channels {
		exists := false
		for _, chann := range updatedChannels {
			if chann == group.id {
				exists = true
				break
			}
		}
		if !exists {
			delete(m.channels, group.id)
			fmt.Printf("Removed channel [Name=%q,ID=%q]\n", group.name, group.id)
		}
	}

	fmt.Println("Channel information updated")

	return nil
}

func (m channelMap) getChannelName(id string) (string, error) {
	m.Lock(); defer m.Unlock()

	if channel, exists := m.channels[id]; !exists {
		return "", errors.New("Channel name not found")
	} else {
		return channel.name, nil
	}
}

type channel struct {
	id string
	name string
	isPublic bool
	sync.Mutex
}

func (c *channel) fetchNewMessages() (err error) {
	c.Lock(); defer c.Unlock()

	fmt.Println("Fetching messages for channel " + c.id)

	var latestMessage *slack.Message

	if c.isPublic {
		info, err := slackApi.GetChannelInfo(c.id)
		if err != nil {
			return err
		}

		latestMessage = info.Latest
	} else {
		info, err := slackApi.GetGroupInfo(c.id)
		if err != nil {
			return err
		}

		latestMessage = info.Latest
	}

	if latestMessage == nil {
		// No messages in this channel
		return nil
	}

	latestInDb, err := db.GetLatestMessageTime(c.id)
	if err != nil {
		return err
	}

	if latestInDb == "" {
		// Set latest time to beginning of time if there are no messages in db
		latestInDb = "0"
	}

	if latestInDb >= latestMessage.Timestamp {
		// Already up-to-date
		fmt.Printf("Messages up-to-date for channel %q\n", c.id)
		return nil
	}

	var history *slack.History
	latestRetrieved := latestInDb

	for {
		params := slack.NewHistoryParameters()
		params.Oldest = latestRetrieved
		params.Inclusive = false
		params.Count = 10
		fmt.Printf("latestRetrieved: %s\n", latestRetrieved)

		if c.isPublic {
			history, err = slackApi.GetChannelHistory(c.id, params)
		} else {
			history, err = slackApi.GetGroupHistory(c.id, params)
		}

		if err != nil {
			return err
		}

		fmt.Printf("Len: %d\n", len(history.Messages))

		messages := make([]db.Message, 0, len(history.Messages))
		for _, newMessage := range history.Messages {
			messages = append(messages, db.Message{newMessage.User, c.id, newMessage.Timestamp, newMessage.Text})

			if newMessage.Timestamp > latestRetrieved {
				latestRetrieved = newMessage.Timestamp
			}
		}
		if err := db.AddMessages(messages); err != nil {
			fmt.Println(err)
			return err
		}

		fmt.Printf("%d messages added from channel %q\n", len(messages), c.id)

		if !history.HasMore {
			break
		}
	}

	return nil
}

func (c *channel) updateEditedMessage(origSendTime string, message string) {
	c.Lock(); defer c.Unlock()
	// TODO implementation
}

func (c *channel) getLatestMessageId() (string, error) {
	c.Lock(); defer c.Unlock()
	return db.GetLatestMessageTime(c.id)
}

func main() {
	defer db.Close()

	var err error

	if ready, err := db.IsReady(); !ready {
		fmt.Printf("Database not ready: %q\n", err)
		os.Exit(1)
	}

	channelInfoMap.init()
	channelInfoMap.update()

	for _, chann := range channelInfoMap.channels {
		chann.fetchNewMessages()
	}
	//channel := channelInfoMap.channels["G3CNTDT8X"]
	//go channel.fetchNewMessages()

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

			// Add new message to database
			db.AddMessages([]db.Message{db.Message{msgData.User, msgData.Channel, msgData.Timestamp, msgData.Text}})
			break
		}
	}
}