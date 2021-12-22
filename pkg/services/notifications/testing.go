package notifications

func (ns *NotificationService) MailQueuePop() *Message {
	return <-ns.mailQueue
}
