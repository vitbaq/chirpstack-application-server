package network

import (
	"encoding/json"
	"fmt"

	"github.com/brocaar/chirpstack-application-server/internal/integration/knot/entities"
	"github.com/cenkalti/backoff/v4"
	log "github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
)

const (
	exchangeTypeDirect = "direct"
	exchangeTypeFanout = "fanout"

	exchangeDevice      = "device"
	ReplyToAuthMessages = "chirpstack-auth-rpc"
)

// AMQP handles the connection, queues and exchanges declared
type AMQP struct {
	url     string
	conn    *amqp.Connection
	channel *amqp.Channel
	queue   *amqp.Queue
}

// InMsg represents the message received from the AMQP broker
type InMsg struct {
	Exchange      string
	RoutingKey    string
	ReplyTo       string
	CorrelationID string
	Headers       map[string]interface{}
	Body          []byte
}

// MessageOptions represents the message publishing options
type MessageOptions struct {
	Authorization string
	CorrelationID string
	ReplyTo       string
	Expiration    string
}

// NewAMQP constructs the AMQP connection handler
func NewAMQP(url string) *AMQP {
	return &AMQP{url, nil, nil, nil}
}

// Start starts the message handling
func (a *AMQP) Start() error {
	err := backoff.Retry(a.connect, backoff.NewExponentialBackOff())
	if err != nil {
		return err
	}

	go a.notifyWhenClosed()
	return nil
}

// Stop closes the connection started
func (a *AMQP) Stop() {
	if a.conn != nil && !a.conn.IsClosed() {
		defer a.conn.Close()
	}

	if a.channel != nil {
		defer a.channel.Close()
	}

	// a.logger.Debug("AMQP handler stopped")
}

// OnMessage receive messages and put them on channel
func (a *AMQP) OnMessage(device chan entities.Device, queueName, exchangeName, exchangeType, key string) error {
	err := a.declareExchange(exchangeName, exchangeType)
	if err != nil {
		// a.logger.Error(err)
		return err
	}

	err = a.declareQueue(queueName)
	if err != nil {
		// a.logger.Error(err)
		return err
	}

	err = a.channel.QueueBind(
		queueName,
		key,
		exchangeName,
		false, // noWait
		nil,   // arguments
	)
	if err != nil {
		// a.logger.Error(err)
		return err
	}

	deliveries, err := a.channel.Consume(
		queueName,
		"",    // consumerTag
		true,  // noAck
		false, // exclusive
		false, // noLocal
		false, // noWait
		nil,   // arguments
	)
	if err != nil {
		// a.logger.Error(err)
		return err
	}

	go convertDeliveryToDeviceMsg(deliveries, device)

	return nil
}

// PublishPersistentMessage sends a persistent message to RabbitMQ
func (a *AMQP) PublishPersistentMessage(exchange, exchangeType, key string, data interface{}, options *MessageOptions) error {
	var headers map[string]interface{}
	var corrID, expTime, replyTo string

	if options != nil {
		headers = map[string]interface{}{
			"Authorization": options.Authorization,
		}
		corrID = options.CorrelationID
		replyTo = options.ReplyTo
		expTime = options.Expiration
	}

	body, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("error enconding JSON message: %w", err)
	}

	err = a.declareExchange(exchange, exchangeType)
	if err != nil {
		return fmt.Errorf("error declaring exchange: %w", err)
	}

	err = a.channel.Publish(
		exchange,
		key,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			Headers:         headers,
			ContentType:     "text/plain",
			ContentEncoding: "",
			DeliveryMode:    amqp.Persistent,
			Priority:        0,
			CorrelationId:   corrID,
			ReplyTo:         replyTo,
			Body:            body,
			Expiration:      expTime,
		},
	)
	if err != nil {
		return fmt.Errorf("error publishing message in channel: %w", err)
	}

	return nil
}

func (a *AMQP) notifyWhenClosed() {
	errReason := <-a.conn.NotifyClose(make(chan *amqp.Error))
	// a.logger.Infof("AMQP connection closed: %s", errReason)
	if errReason != nil {
		err := backoff.Retry(a.connect, backoff.NewExponentialBackOff())
		if err != nil {
			// a.logger.Error(err)
			return
		}

		go a.notifyWhenClosed()
	}
}

func (a *AMQP) connect() error {
	conn, err := amqp.Dial(a.url)
	if err != nil {
		// a.logger.Error(err)
		return err
	}

	a.conn = conn
	channel, err := a.conn.Channel()
	if err != nil {
		// a.logger.Error(err)
		return err
	}

	// a.logger.Debug("AMQP handler connected")
	a.channel = channel

	return nil
}

func (a *AMQP) declareExchange(name, exchangeType string) error {
	return a.channel.ExchangeDeclare(
		name,
		exchangeType, // type
		true,         // durable
		false,        // delete when complete
		false,        // internal
		false,        // noWait
		nil,          // arguments
	)
}

func (a *AMQP) declareQueue(name string) error {
	queue, err := a.channel.QueueDeclare(
		name,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // noWait
		nil,   // arguments
	)

	a.queue = &queue
	return err
}

func convertDeliveryToDeviceMsg(deliveries <-chan amqp.Delivery, deviceChan chan entities.Device) {

	for d := range deliveries {

		switch d.RoutingKey {

		// registered msg from knot
		case BindingKeyRegistered:

			deviceInf := entities.Device{}

			Receiver := DeviceRegisteredResponse{}

			json.Unmarshal([]byte(string(d.Body)), &Receiver)
			deviceInf.ID = Receiver.ID
			deviceInf.Name = Receiver.Name

			if Receiver.Error != "" {
				//alread registered
				deviceInf.Error = Receiver.Error
				deviceInf.State = entities.KnotError
				deviceChan <- deviceInf
			} else {
				deviceInf.Token = Receiver.Token
				deviceInf.State = entities.KnotRegistered
				deviceChan <- deviceInf
			}
			log.WithFields(log.Fields{"amqp": "knot"}).Info("received a registration response")

		// unregistered
		case BindingKeyUnregistered:

			deviceInf := entities.Device{}

			Receiver := DeviceUnregisterRequest{}

			json.Unmarshal([]byte(string(d.Body)), &Receiver)
			deviceInf.ID = Receiver.ID
			deviceInf.State = entities.KnotDelete
			deviceChan <- deviceInf

			log.WithFields(log.Fields{"amqp": "knot"}).Info("received a unregistration response")

		//receive a auth msg
		case ReplyToAuthMessages:
			deviceInf := entities.Device{}

			Receiver := DeviceAuthResponse{}

			json.Unmarshal([]byte(string(d.Body)), &Receiver)
			deviceInf.ID = Receiver.ID

			if Receiver.Error != "" {
				//alread registered
				deviceInf.Error = Receiver.Error
				deviceInf.State = entities.KnotError
				deviceChan <- deviceInf
				log.WithFields(log.Fields{"amqp": "knot"}).Info("received a error on authentication response")
			} else {
				deviceInf.State = entities.KnotAuth
				deviceChan <- deviceInf
				log.WithFields(log.Fields{"amqp": "knot"}).Info("received a authentication response")
			}
		case BindingKeyUpdatedConfig:
			deviceInf := entities.Device{}

			Receiver := ConfigUpdatedResponse{}

			json.Unmarshal([]byte(string(d.Body)), &Receiver)
			deviceInf.ID = Receiver.ID

			if Receiver.Error != "" {
				//alread registered
				deviceInf.Error = Receiver.Error
				deviceInf.State = entities.KnotError
				deviceChan <- deviceInf
				log.WithFields(log.Fields{"amqp": "knot"}).Info("received a error on config update response")
			} else {
				deviceInf.State = entities.KnotoK
				deviceChan <- deviceInf
				log.WithFields(log.Fields{"amqp": "knot"}).Info("received a config update response")
			}
		}

		//outMsg <- InMsg{d.Exchange, d.RoutingKey, d.ReplyTo, d.CorrelationId, d.Headers, d.Body}
	}
}
