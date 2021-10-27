package network

import "github.com/brocaar/chirpstack-application-server/internal/integration/knot/entities"

const (
	queueName = "chirpstack-knot-messages"

	BindingKeyRegistered    = "device.registered"
	BindingKeyUnregistered  = "device.unregistered"
	BindingKeyUpdatedConfig = "device.config.updated"
)

// Subscriber provides methods to subscribe to events on message broker
type Subscriber interface {
	SubscribeToKNoTMessages(deviceChan chan entities.Device) error
}

type msgSubscriber struct {
	amqp *AMQP
}

// NewMsgSubscriber constructs the msgSubscriber
func NewMsgSubscriber(amqp *AMQP) Subscriber {
	return &msgSubscriber{amqp}
}

func (ms *msgSubscriber) SubscribeToKNoTMessages(deviceChan chan entities.Device) error {
	var err error

	subscribe := func(deviceChan chan entities.Device, queue, exchange, kind, key string) error {
		err = ms.amqp.OnMessage(deviceChan, queue, exchange, kind, key)
		if err != nil {
			return err
		}
		return nil
	}

	err = subscribe(deviceChan, queueName, exchangeDevice, exchangeTypeDirect, BindingKeyRegistered)
	if err != nil {
		return err
	}

	err = subscribe(deviceChan, queueName, exchangeDevice, exchangeTypeDirect, BindingKeyUnregistered)
	if err != nil {
		return err
	}

	err = subscribe(deviceChan, queueName, exchangeDevice, exchangeTypeDirect, ReplyToAuthMessages)
	if err != nil {
		return err
	}

	err = subscribe(deviceChan, queueName, exchangeDevice, exchangeTypeDirect, BindingKeyUpdatedConfig)
	if err != nil {
		return err
	}

	return nil
}
