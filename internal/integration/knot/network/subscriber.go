package network

const (
	queueName = "chirpstack-knot-messages"

	bindingKeyRegistered    = "device.registered"
	bindingKeyUnregistered  = "device.unregistered"
	bindingKeyUpdatedConfig = "device.config.updated"
)

// Subscriber provides methods to subscribe to events on message broker
type Subscriber interface {
	SubscribeToKNoTMessages(msgChan chan InMsg) error
}

type msgSubscriber struct {
	amqp *AMQP
}

// NewMsgSubscriber constructs the msgSubscriber
func NewMsgSubscriber(amqp *AMQP) Subscriber {
	return &msgSubscriber{amqp}
}

func (ms *msgSubscriber) SubscribeToKNoTMessages(msgChan chan InMsg) error {
	var err error
	subscribe := func(msgChan chan InMsg, queue, exchange, kind, key string) {
		if err != nil {
			return
		}
		err = ms.amqp.OnMessage(msgChan, queue, exchange, kind, key)
	}

	subscribe(msgChan, queueName, exchangeDevice, exchangeTypeDirect, bindingKeyRegistered)
	subscribe(msgChan, queueName, exchangeDevice, exchangeTypeDirect, bindingKeyUnregistered)
	subscribe(msgChan, queueName, exchangeDevice, exchangeTypeDirect, replyToAuthMessages)
	subscribe(msgChan, queueName, exchangeDevice, exchangeTypeDirect, bindingKeyUpdatedConfig)

	return nil
}
