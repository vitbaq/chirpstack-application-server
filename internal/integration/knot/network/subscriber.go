package network

const (
	queueName = "chirpstack-knot-messages"

	// Binding Keys used to receive knot cloud data
	BindingKeyRegistered    = "device.registered"
	BindingKeyUnregistered  = "device.unregistered"
	BindingKeyUpdatedConfig = "device.config.updated"
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

// SubscribeToKNoTMessages register AMQP to receive data from Knot Cloud
func (ms *msgSubscriber) SubscribeToKNoTMessages(msgChan chan InMsg) error {
	var err error
	subscribe := func(msgChan chan InMsg, queue, exchange, kind, key string) {
		if err != nil {
			return
		}
		err = ms.amqp.OnMessage(msgChan, queue, exchange, kind, key)
	}

	subscribe(msgChan, queueName, exchangeDevice, exchangeTypeDirect, BindingKeyRegistered)
	subscribe(msgChan, queueName, exchangeDevice, exchangeTypeDirect, BindingKeyUnregistered)
	subscribe(msgChan, queueName, exchangeDevice, exchangeTypeDirect, ReplyToAuthMessages)
	subscribe(msgChan, queueName, exchangeDevice, exchangeTypeDirect, BindingKeyUpdatedConfig)

	return nil
}
