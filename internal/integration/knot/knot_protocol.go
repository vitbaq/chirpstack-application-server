package knot

import (
	"fmt"

	"github.com/brocaar/chirpstack-application-server/internal/config"
	"github.com/brocaar/chirpstack-application-server/internal/integration/knot/entities"
	"github.com/brocaar/chirpstack-application-server/internal/integration/knot/network"
	log "github.com/sirupsen/logrus"
)

// Protocol interface provides methods to handle KNoT Protocol
type Protocol interface {
	Close() error
	CreateDevice(device entities.Device) error
	DeleteDevice(id string) error
}

type networkWrapper struct {
	amqp       *network.AMQP
	publisher  network.Publisher
	subscriber network.Subscriber
}

type protocol struct {
	userToken string
	network   *networkWrapper
	devices   map[string]entities.Device
}

func newProtocol(conf config.IntegrationKNoTConfig, msgChan chan entities.Device) (Protocol, error) {
	p := &protocol{}

	p.userToken = conf.UserToken
	p.network = new(networkWrapper)
	p.network.amqp = network.NewAMQP(conf.URL)

	err := p.network.amqp.Start()
	if err != nil {
		log.WithFields(log.Fields{
			"integration": "knot",
			"connection":  "error",
		}).Info("Error connection to knot-cloud: %v", err)
		return p, err
	}

	p.network.publisher = network.NewMsgPublisher(p.network.amqp)
	p.network.subscriber = network.NewMsgSubscriber(p.network.amqp)

	log.WithFields(log.Fields{
		"integration": "knot",
	}).Info("New connection to knot-cloud")

	p.devices = make(map[string]entities.Device)

	go ControlData(msgChan, p)

	return p, nil
}

// Close closes the protocol.
func (p *protocol) Close() error {
	p.network.amqp.Stop()
	return nil
}

func (p *protocol) CreateDevice(device entities.Device) error {
	if _, d := p.devices[device.ID]; d {

		log.WithFields(log.Fields{
			"dev_eui": device.ID,
		}).Info("Device already exist")

		return fmt.Errorf("Device already exist")
	}

	log.WithFields(log.Fields{
		"dev_eui": device.ID,
	}).Info("Device created")

	p.devices[device.ID] = device

	return nil
}

func (p *protocol) DeleteDevice(id string) error {
	if _, d := p.devices[id]; !d {
		return fmt.Errorf("Device do not exist")
	}

	delete(p.devices, id)
	return nil
}

func ControlData(msgChan chan entities.Device, p *protocol) {
	for device := range msgChan {
		switch device.State {

		case entities.KnotNew:
			p.CreateDevice(device)
		}
	}
}
