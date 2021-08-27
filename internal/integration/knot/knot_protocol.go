package knot

import (
	"fmt"

	"github.com/brocaar/chirpstack-application-server/internal/config"
	"github.com/brocaar/chirpstack-application-server/internal/integration/knot/entities"
	"github.com/brocaar/chirpstack-application-server/internal/integration/knot/network"
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

func newProtocol(conf config.IntegrationKNoTConfig) (Protocol, error) {
	p := &protocol{}

	p.userToken = conf.UserToken

	p.network.amqp = network.NewAMQP(conf.URL)
	p.network.amqp.Start()

	p.network.publisher = network.NewMsgPublisher(p.network.amqp)
	p.network.subscriber = network.NewMsgSubscriber(p.network.amqp)

	return p, nil
}

// Close closes the protocol.
func (p *protocol) Close() error {
	p.network.amqp.Stop()
	return nil
}

func (p *protocol) CreateDevice(device entities.Device) error {
	if _, d := p.devices[device.ID]; d {
		return fmt.Errorf("Device already exist")
	}

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
