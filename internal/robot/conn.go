package robot

// MessageHandler 供Feishu/DingTalk长连接调用的message处理接口（由 handler.RobotHandler 实现）
type MessageHandler interface {
	HandleMessage(platform, userID, text string) string
}
