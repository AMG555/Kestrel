package robot

// MessageHandler is the message-handling interface for Feishu/DingTalk long-lived connections
// (implemented by handler.RobotHandler).
type MessageHandler interface {
	HandleMessage(platform, userID, text string) string
}
