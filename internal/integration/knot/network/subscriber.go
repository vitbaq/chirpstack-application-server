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

	subscribe := func(msgChan chan InMsg, queue, exchange, kind, key string) error {
		err = ms.amqp.OnMessage(msgChan, queue, exchange, kind, key)
		if err != nil {
			return err
		}
		return nil
	}

	err = subscribe(msgChan, queueName, exchangeDevice, exchangeTypeDirect, BindingKeyRegistered)
	if err != nil {
		return err
	}

	err = subscribe(msgChan, queueName, exchangeDevice, exchangeTypeDirect, BindingKeyUnregistered)
	if err != nil {
		return err
	}

	err = subscribe(msgChan, queueName, exchangeDevice, exchangeTypeDirect, ReplyToAuthMessages)
	if err != nil {
		return err
	}

	err = subscribe(msgChan, queueName, exchangeDevice, exchangeTypeDirect, BindingKeyUpdatedConfig)
	if err != nil {
		return err
	}

	return nil
}
