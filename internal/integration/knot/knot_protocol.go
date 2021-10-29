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
	UpdateDevice(device entities.Device) error
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

func newProtocol(conf config.IntegrationKNoTConfig, DeviceChan chan entities.Device) (Protocol, error) {
	p := &protocol{}

	p.userToken = conf.UserToken
	p.network = new(networkWrapper)
	p.network.amqp = network.NewAMQP(conf.URL)

	err := p.network.amqp.Start()
	if err != nil {
		log.WithFields(log.Fields{"integration": "knot"}).Info("Error connection to knot-cloud: %v", err)
		return p, err
	} else {
		log.WithFields(log.Fields{"integration": "knot"}).Info("New connection to knot-cloud")
	}

	p.network.publisher = network.NewMsgPublisher(p.network.amqp)
	p.network.subscriber = network.NewMsgSubscriber(p.network.amqp)

	if err = p.network.subscriber.SubscribeToKNoTMessages(DeviceChan); err != nil {
		log.WithFields(log.Fields{"integration": "knot"}).Info("Error to subscribe")
		return p, err
	}

	p.devices = make(map[string]entities.Device)

	go ControlData(DeviceChan, p)

	return p, nil
}

func (p *protocol) UpdateDevice(device entities.Device) error {

	if _, d := p.devices[device.ID]; !d {
		//se não existir, retorna false
		log.WithFields(log.Fields{"debug": "true", "Update Device": "Error"}).Info("Device do not exist")
		return fmt.Errorf("Device do not exist")
	}

	receiver := p.devices[device.ID]
	if device.Name != "" {
		receiver.Name = device.Name
	}
	if device.Token != "" {
		receiver.Token = device.Token
	}
	if device.State != "" {
		receiver.State = device.State
	}
	if device.Error != "" {
		receiver.Error = device.Error
	}
	p.devices[device.ID] = receiver
	return nil
}

// Close closes the protocol.
func (p *protocol) Close() error {
	p.network.amqp.Stop()
	return nil
}

func (p *protocol) CreateDevice(device entities.Device) error {

	if _, d := p.devices[device.ID]; d {

		p.UpdateDevice(device)
		//log.WithFields(log.Fields{"dev_eui": device.ID}).Info("Device already exist")

		return fmt.Errorf("Device already exist")
	}

	log.WithFields(log.Fields{"dev_eui": device.ID}).Info("Device created")

	device.State = entities.KnotNew

	p.devices[device.ID] = device
	p.UpdateDevice(device)

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

		// creates a new device if it doesn't exist
		p.CreateDevice(device)
		device = p.devices[device.ID]

		switch device.State {

		// if the device status is new, request a device registration
		case entities.KnotNew:

			device.State = entities.KnotRegisterReq
			p.UpdateDevice(device)
			log.WithFields(log.Fields{"knot": entities.KnotNew}).Info("send a register request")
			p.network.publisher.PublishDeviceRegister(p.userToken, &device)

		// if the device is already registered, ask for device authentication
		case entities.KnotRegistered:

			log.WithFields(log.Fields{"knot": entities.KnotRegistered}).Info("send a auth request")
			p.network.publisher.PublishDeviceAuth(p.userToken, &device)

		// check if the device has a token, if it does, delete it, if not, resend the registration request
		case entities.KnotDelete:

			if device.Token != "" {
				log.WithFields(log.Fields{"knot": entities.KnotDelete}).Info("delete a device")
				p.DeleteDevice(device.ID)
			} else {
				device.State = entities.KnotRegisterReq
				p.UpdateDevice(device)
				log.WithFields(log.Fields{"knot": entities.KnotDelete}).Info("send a register request")
				p.network.publisher.PublishDeviceRegister(p.userToken, &device)
			}

		// just delete
		case entities.KnotForceDelete:

			log.WithFields(log.Fields{"knot": entities.KnotForceDelete}).Info("delete a device")
			p.DeleteDevice(device.ID)

		// handle errors
		case entities.KnotError:
			switch device.Error {

			//if the device is new to the chirpstack platform, but already has a registration in Knot, first the device needs to ask to unregister and then ask for a registration.
			case "thing is already registered":
				log.WithFields(log.Fields{"knot": entities.KnotError}).Info("device is registered, but does not have a token; send a unrequest request")
				p.network.publisher.PublishDeviceUnregister(p.userToken, &device)

			default:
				log.WithFields(log.Fields{"knot": entities.KnotError}).Info("ERROR WITHOUT HANDLER" + device.Error)

			}
		}
	}
}
