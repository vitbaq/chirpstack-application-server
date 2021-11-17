package knot

import (
	"encoding/json"
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

// Create a new connection with knot cloud
func newProtocol(conf config.IntegrationKNoTConfig, deviceChan chan entities.Device, msgChan chan network.InMsg) (Protocol, error) {
	p := &protocol{}

	p.userToken = conf.UserToken
	p.network = new(networkWrapper)
	p.network.amqp = network.NewAMQP(conf.URL)

	err := p.network.amqp.Start()
	if err != nil {
		log.WithFields(log.Fields{"integration": "knot"}).Error("Error connection to knot-cloud: %v", err)
		return p, err
	} else {
		log.WithFields(log.Fields{"integration": "knot"}).Error("New connection to knot-cloud")
	}

	p.network.publisher = network.NewMsgPublisher(p.network.amqp)
	p.network.subscriber = network.NewMsgSubscriber(p.network.amqp)

	if err = p.network.subscriber.SubscribeToKNoTMessages(msgChan); err != nil {
		log.WithFields(log.Fields{"integration": "knot"}).Error("Error to subscribe")
		return p, err
	}

	p.devices = make(map[string]entities.Device)

	go handlerKnotAMQP(msgChan, deviceChan)
	go DataControl(deviceChan, p)

	return p, nil
}

// Update the knot device information on map
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

// Create a new knot device
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

// Delete the knot device from map
func (p *protocol) DeleteDevice(id string) error {
	if _, d := p.devices[id]; !d {
		return fmt.Errorf("Device do not exist")
	}

	delete(p.devices, id)
	return nil
}

// Control device paths.
func DataControl(deviceChan chan entities.Device, p *protocol) {
	for device := range deviceChan {

		// Creates a new device if it doesn't exist
		p.CreateDevice(device)
		device = p.devices[device.ID]

		switch device.State {

		// If the device status is new, request a device registration
		case entities.KnotNew:

			device.State = entities.KnotRegisterReq
			p.UpdateDevice(device)
			log.WithFields(log.Fields{"knot": entities.KnotNew}).Info("send a register request")
			p.network.publisher.PublishDeviceRegister(p.userToken, &device)

		// If the device is already registered, ask for device authentication
		case entities.KnotRegistered:

			log.WithFields(log.Fields{"knot": entities.KnotRegistered}).Info("send a auth request")
			p.network.publisher.PublishDeviceAuth(p.userToken, &device)

		// Now the device has a token, make a new request for authentication.
		case entities.KnotAuth:

			log.WithFields(log.Fields{"knot": entities.KnotAuth}).Info("send the new configuration")
			p.network.publisher.PublishDeviceUpdateConfig(p.userToken, &device)

		// Check if the device has a token, if it does, delete it, if not, resend the registration request
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

		// Just delete
		case entities.KnotForceDelete:

			log.WithFields(log.Fields{"knot": entities.KnotForceDelete}).Info("delete a device")
			p.DeleteDevice(device.ID)

		// Handle errors
		case entities.KnotError:
			switch device.Error {
			// If the device is new to the chirpstack platform, but already has a registration in Knot, first the device needs to ask to unregister and then ask for a registration.
			case "thing is already registered":
				log.WithFields(log.Fields{"knot": entities.KnotError}).Error("device is registered, but does not have a token; send a unrequest request")
				p.network.publisher.PublishDeviceUnregister(p.userToken, &device)

			default:
				log.WithFields(log.Fields{"knot": entities.KnotError}).Error("ERROR WITHOUT HANDLER" + device.Error)

			}
		}
	}
}

// Just formart ther Error msg
func errorFormat(device entities.Device, strError string) entities.Device {
	device.Error = strError
	device.State = entities.KnotError
	log.WithFields(log.Fields{"amqp": "knot"}).Error(strError)
	return device
}

// Handles messages coming from AMQP
func handlerKnotAMQP(msgChan <-chan network.InMsg, deviceChan chan entities.Device) {

	for d := range msgChan {

		switch d.RoutingKey {

		// Registered msg from knot
		case network.BindingKeyRegistered:

			deviceInf := entities.Device{}

			receiver := network.DeviceRegisteredResponse{}

			json.Unmarshal([]byte(string(d.Body)), &receiver)
			deviceInf.ID = receiver.ID
			deviceInf.Name = receiver.Name

			if receiver.Error != "" {
				// Alread registered
				deviceChan <- errorFormat(deviceInf, receiver.Error)
			} else {
				deviceInf.Token = receiver.Token
				deviceInf.State = entities.KnotRegistered
				deviceChan <- deviceInf
			}
			log.WithFields(log.Fields{"amqp": "knot"}).Info("received a registration response")

		// Unregistered
		case network.BindingKeyUnregistered:

			deviceInf := entities.Device{}

			receiver := network.DeviceUnregisterRequest{}

			json.Unmarshal([]byte(string(d.Body)), &receiver)
			deviceInf.ID = receiver.ID
			deviceInf.State = entities.KnotDelete
			deviceChan <- deviceInf
			log.WithFields(log.Fields{"amqp": "knot"}).Info("received a unregistration response")

		// Receive a auth msg
		case network.ReplyToAuthMessages:
			deviceInf := entities.Device{}

			receiver := network.DeviceAuthResponse{}

			json.Unmarshal([]byte(string(d.Body)), &receiver)
			deviceInf.ID = receiver.ID

			if receiver.Error != "" {
				// Alread registered
				deviceChan <- errorFormat(deviceInf, receiver.Error)
			} else {
				deviceInf.State = entities.KnotAuth
				deviceChan <- deviceInf
				log.WithFields(log.Fields{"amqp": "knot"}).Info("received a authentication response")
			}
		case network.BindingKeyUpdatedConfig:
			deviceInf := entities.Device{}

			receiver := network.ConfigUpdatedResponse{}

			json.Unmarshal([]byte(string(d.Body)), &receiver)
			deviceInf.ID = receiver.ID
			fmt.Println("------------------------------------------------------------")
			fmt.Println(receiver)
			fmt.Println("------------------------------------------------------------")
			if receiver.Error != "" {
				// Alread registered
				deviceChan <- errorFormat(deviceInf, receiver.Error)
			} else {
				deviceInf.State = entities.KnotoK
				deviceChan <- deviceInf
				log.WithFields(log.Fields{"amqp": "knot"}).Info("received a config update response")
			}
		}
	}
}
