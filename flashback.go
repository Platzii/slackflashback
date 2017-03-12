package main

import (
	"earthgrazer.ca/slackflashback/db"
	"errors"
	"flag"
	"fmt"
	"github.com/nlopes/slack"
	log "github.com/sirupsen/logrus"
	"os"
	"strings"
	"sync"
)

// Configurable parameters
var (
	authToken *string
	botName   *string
)

var (
	botId    string
	slackApi *slack.Client
	search   db.Search
)

var (
	userIdNameMap  map[string]string = make(map[string]string)
	channelInfoMap channelMap
)

func init() {
	log.SetOutput(os.Stdout)
	log.SetLevel(log.DebugLevel)

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
	m.Lock()
	defer m.Unlock()

	updatedChannels := []string{}

	log.Info("Updating channel information...")

	groups, err := slackApi.GetGroups(true)
	if err != nil {
		return err
	}

	// Add non-existing private channels that this bot belongs to
	for _, group := range groups {
		updatedChannels = append(updatedChannels, group.ID)
		_, exists := m.channels[group.ID]

		if !exists {
			m.channels[group.ID] = channel{
				id:       group.ID,
				name:     group.Name,
				isPublic: false,
			}
			log.Debugf("Added private channel [Name=%q,ID=%q]", group.Name, group.ID)
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
			m.channels[chann.ID] = channel{
				id:       chann.ID,
				name:     chann.Name,
				isPublic: true,
			}
			log.Debugf("Added public channel [Name=%q,ID=%q]", chann.Name, chann.ID)
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
			log.Debugf("Removed channel [Name=%q,ID=%q]", group.name, group.id)
		}
	}

	log.Infof("Channel information updated")

	return nil
}

func (m channelMap) getChannelName(id string) (string, error) {
	m.Lock()
	defer m.Unlock()

	if channel, exists := m.channels[id]; !exists {
		return "", errors.New("Channel name not found")
	} else {
		return channel.name, nil
	}
}

type channel struct {
	id       string
	name     string
	isPublic bool
	sync.Mutex
}

func (c *channel) fetchNewMessages() (err error) {
	c.Lock()
	defer c.Unlock()

	log.Debugf("Fetching messages for channel " + c.id)

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
		latestInDb = "1"
	}

	if latestInDb >= latestMessage.Timestamp {
		// Already up-to-date
		log.Debugf("Messages up-to-date for channel %q", c.id)
		return nil
	}

	var history *slack.History
	var messageHistory []db.Message
	latestRetrieved := latestInDb

	for {
		params := slack.NewHistoryParameters()
		params.Oldest = latestRetrieved
		params.Inclusive = false
		params.Count = 100

		if c.isPublic {
			history, err = slackApi.GetChannelHistory(c.id, params)
		} else {
			history, err = slackApi.GetGroupHistory(c.id, params)
		}

		if err != nil {
			return err
		}

		messages := make([]db.Message, 0, len(history.Messages))
		for _, newMessage := range history.Messages {
			if newMessage.Timestamp > latestRetrieved {
				latestRetrieved = newMessage.Timestamp
			}

			if newMessage.BotID != "" || newMessage.User == botId {
				// Ignore any messages from bots
				continue
			}

			if search.IsCommand(newMessage.Text) {
				// Ignore any commands to this bot
				continue
			}

			messages = append(messages, db.Message{Sender: newMessage.User, Channel: c.id, SendTime: newMessage.Timestamp, Message: newMessage.Text})
		}
		messageHistory = append(messageHistory, messages...)

		if !history.HasMore {
			break
		}
	}

	if err := db.AddMessages(messageHistory); err != nil {
		fmt.Println(err)
		return err
	}

	log.Debugf("%d messages added from channel %q", len(messageHistory), c.id)

	return nil
}

func (c *channel) updateEditedMessage(origSendTime string, message string) {
	c.Lock()
	defer c.Unlock()
	// TODO implementation
}

func (c *channel) getLatestMessageId() (string, error) {
	c.Lock()
	defer c.Unlock()
	return db.GetLatestMessageTime(c.id)
}

func resolveUserMapping() error {
	var users []slack.User

	log.Info("Resolving user mapping...")
	users, err := slackApi.GetUsers()

	if err != nil {
		return err
	}

	for _, user := range users {
		if _, ok := userIdNameMap[user.ID]; !ok {
			userIdNameMap[user.ID] = user.Name
		}

		if user.Name == getBotName() {
			botId = user.ID
			log.Infof("Bot id: %s", botId)
		}
	}

	for id := range userIdNameMap {
		exists := false
		for _, user := range users {
			if user.ID == id {
				exists = true
				break
			}
		}
		if !exists {
			delete(userIdNameMap, id)
		}
	}

	if botId == "" {
		return errors.New("Bot id not found")
	}
	log.Infof("Finished resolving user mapping. Total users found=%d", len(userIdNameMap))
	return nil
}

func handleNewMessage(msgEv *slack.MessageEvent) error {
	if msgEv.BotID != "" || msgEv.User == botId {
		// Ignore any messages from bots
		return nil
	}

	log.Debugf("New message received from user %q in channel %q: %q", msgEv.User, msgEv.Channel, msgEv.Text)

	if err := handleCommand(msgEv); err != nil {
		log.Errorf("Error encountered while handling command: %s", err)
	}

	if err := checkNewMessagesForChannel(msgEv.Channel); err != nil {
		return err
	}

	return nil
}

func handleCommand(msgEv *slack.MessageEvent) error {
	if !search.IsCommand(msgEv.Text) {
		log.Debug("Received non-command message")
		return nil
	}

	log.Debug("Received command message")

	query, err := search.GetQueryFromCommand(msgEv.Text)
	if err != nil {
		return err
	}

	results, err := db.SearchMessage("", msgEv.Channel, query)
	if err != nil {
		return err
	}

	resultMsgs := make([]string, 0, len(results))
	for _, result := range results {
		sendTime, err := ConvertTimestampToString(result.Msg.SendTime)
		if err != nil {
			log.Error("Error parsing message send time")
			sendTime = ""
		}
		userName, ok := userIdNameMap[result.Msg.Sender]
		if !ok {
			log.Errorf("Message sender name not found: %q", result.Msg.Sender)
			userName = ""
		}
		processedMsg := SubstituteUserIdWithName(userIdNameMap, result.Msg.Message)
		resultMsgs = append(resultMsgs, fmt.Sprintf("*%s posted on %s:* %s", userName, sendTime, processedMsg))
	}
	resultStr := strings.Join(resultMsgs, "\n")

	uploadParams := slack.FileUploadParameters{
		Content:  resultStr,
		Filetype: "post",
		Channels: []string{msgEv.Channel},
		Filename: "Message search results"}
	slackApi.UploadFile(uploadParams)

	return nil
}

func checkNewMessagesForChannel(channName string) error {
	// Check if this is a new channel that we didn't know about. Fetch past messages if it is new.
	if _, hasChannel := channelInfoMap.channels[channName]; !hasChannel {
		channelInfoMap.update()
	}

	if chann, hasChannel := channelInfoMap.channels[channName]; !hasChannel {
		return errors.New(fmt.Sprintf("Unable to get mapping for new channel %q", channName))
	} else {
		chann.fetchNewMessages()
	}

	return nil
}

func main() {
	defer db.Close()

	if ready, err := db.IsReady(); !ready {
		log.Fatalf("Database not ready: %q", err)
		os.Exit(1)
	}

	err := resolveUserMapping()
	if err != nil {
		log.Fatalf("Error resolving user mapping: %q", err)
		os.Exit(1)
	}

	if err := search.SetBotInfo(botId, userIdNameMap[botId]); err != nil {
		log.Fatalf("Error setting bot id: %q", botId)
		os.Exit(1)
	}
	search.SetUserMap(userIdNameMap)

	channelInfoMap.init()
	channelInfoMap.update()

	for _, chann := range channelInfoMap.channels {
		chann.fetchNewMessages()
	}

	rtm := slackApi.NewRTM()

	go rtm.ManageConnection()

	// Start processing new messages from all channels that the bot belongs to
	// Loop until interrupted
	for msg := range rtm.IncomingEvents {
		switch msgData := msg.Data.(type) {
		case *slack.MessageEvent:
			go handleNewMessage(msgData)
		}
	}
}
