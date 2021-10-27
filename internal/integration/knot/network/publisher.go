package network

import (
	"github.com/brocaar/chirpstack-application-server/internal/integration/knot/entities"
)

const (
	routingKeyRegister     = "device.register"
	routingKeyUnregister   = "device.unregister"
	routingKeyAuth         = "device.auth"
	routingKeyUpdateConfig = "device.config.sent"
	routingKeyDataSent     = "data.sent"

	defaultCorrelationID = "default-corrId"

	defaultExpirationTime = "2000"
)

// Publisher provides methods to subscribe to events on message broker
type Publisher interface {
	PublishDeviceRegister(userToken string, device *entities.Device) error
	PublishDeviceUnregister(userToken string, device *entities.Device) error
	PublishDeviceAuth(userToken string, device *entities.Device) error
	PublishDeviceUpdateConfig(userToken string, device *entities.Device) error
	PublishDeviceData(userToken string, device *entities.Device, data []entities.Data) error
}

type msgPublisher struct {
	amqp *AMQP
}

// NewMsgPublisher constructs the msgPublisher
func NewMsgPublisher(amqp *AMQP) Publisher {
	return &msgPublisher{amqp}
}

func (mp *msgPublisher) PublishDeviceRegister(userToken string, device *entities.Device) error {
	options := MessageOptions{
		Authorization: userToken,
		Expiration:    defaultExpirationTime,
	}

	message := DeviceRegisterRequest{
		ID:   device.ID,
		Name: device.Name,
	}

	err := mp.amqp.PublishPersistentMessage(exchangeDevice, exchangeTypeDirect, routingKeyRegister, message, &options)
	if err != nil {
		return err
	}

	return nil
}

func (mp *msgPublisher) PublishDeviceUnregister(userToken string, device *entities.Device) error {
	options := MessageOptions{
		Authorization: userToken,
		Expiration:    defaultExpirationTime,
	}

	message := DeviceUnregisterRequest{
		ID: device.ID,
	}

	err := mp.amqp.PublishPersistentMessage(exchangeDevice, exchangeTypeDirect, routingKeyUnregister, message, &options)
	if err != nil {
		return err
	}

	return nil
}

func (mp *msgPublisher) PublishDeviceAuth(userToken string, device *entities.Device) error {
	options := MessageOptions{
		Authorization: userToken,
		Expiration:    defaultExpirationTime,
		CorrelationID: defaultCorrelationID,
		ReplyTo:       ReplyToAuthMessages,
	}

	message := DeviceAuthRequest{
		ID:    device.ID,
		Token: device.Token,
	}

	err := mp.amqp.PublishPersistentMessage(exchangeDevice, exchangeTypeDirect, routingKeyAuth, message, &options)
	if err != nil {
		return err
	}

	return nil
}

func (mp *msgPublisher) PublishDeviceUpdateConfig(userToken string, device *entities.Device) error {
	options := MessageOptions{
		Authorization: userToken,
		Expiration:    defaultExpirationTime,
	}

	message := ConfigUpdateRequest{
		ID:     device.ID,
		Config: device.Config,
	}

	err := mp.amqp.PublishPersistentMessage(exchangeDevice, exchangeTypeDirect, routingKeyUpdateConfig, message, &options)
	if err != nil {
		return err
	}

	return nil
}

func (mp *msgPublisher) PublishDeviceData(userToken string, device *entities.Device, data []entities.Data) error {
	options := MessageOptions{
		Authorization: userToken,
		Expiration:    defaultExpirationTime,
	}

	message := DataSent{
		ID:   device.ID,
		Data: data,
	}

	err := mp.amqp.PublishPersistentMessage(exchangeDevice, exchangeTypeFanout, routingKeyDataSent, message, &options)
	if err != nil {
		return err
	}

	return nil
}
