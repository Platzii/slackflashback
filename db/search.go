package db

type command string

type Search struct {
	userMap map[string]string
	botId string
	botName string
}

func (s *Search) SetBotInfo(botId string, botName string) {
	s.botId = botId
	s.botName = botName
}

func (s *Search) SetUserMap(userMap map[string]string) {
	s.userMap = userMap
}

func (s *Search) GetQueryFromCommand(cmd string) (string, error) {
	return "", nil
}

func (s *Search) isCommand(command string) bool {
	return false
}
