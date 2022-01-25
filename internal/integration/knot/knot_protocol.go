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
	createDevice(device entities.Device) error
	deleteDevice(id string) error
	updateDevice(device entities.Device) error
	checkData(device entities.Device) error
	checkDeviceConfiguration(device entities.Device) error
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
	go dataControl(deviceChan, p)

	return p, nil
}

// Check for data to be updated
func (p *protocol) checkData(device entities.Device) error {
	var ok bool
	id_pass := 0
	// Check if the ids are correct, no repetition
	for _, data := range device.Data {
		if data.SensorID != id_pass {
			id_pass = data.SensorID
			ok = true
		} else {
			ok = false
		}
	}
	if ok {
		return nil
	}
	return fmt.Errorf("Invalid Data")
}

// Check for device configuration
func (p *protocol) checkDeviceConfiguration(device entities.Device) error {
	var ok bool
	id_pass := 0
	// Check if the ids are correct, no repetition
	for _, data := range device.Config {
		if data.SensorID != id_pass {
			id_pass = data.SensorID
			ok = true
		} else {
			ok = false
		}
	}
	if ok {
		return nil
	}
	return fmt.Errorf("Invalid Config")
}

// Update the knot device information on map
func (p *protocol) updateDevice(device entities.Device) error {

	if _, checkDevice := p.devices[device.ID]; !checkDevice {

		log.WithFields(log.Fields{"debug": "true", "Update Device": "Error"}).Error("Device do not exist")
		return fmt.Errorf("Device do not exist")
	}

	receiver := p.devices[device.ID]

	if p.checkDeviceConfiguration(device) == nil {
		receiver.Config = device.Config
	}
	if p.checkData(device) == nil {
		receiver.Data = device.Data
	}
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
func (p *protocol) createDevice(device entities.Device) error {

	if _, checkDevice := p.devices[device.ID]; checkDevice {

		err := p.updateDevice(device)
		if err != nil {
			return err
		}
		return fmt.Errorf("Device already exist")
	}

	log.WithFields(log.Fields{"dev_eui": device.ID}).Info("Device created")

	device.State = entities.KnotNew

	p.devices[device.ID] = device
	err := p.updateDevice(device)
	if err != nil {
		return err
	}
	return nil
}

// Delete the knot device from map
func (p *protocol) deleteDevice(id string) error {
	if _, d := p.devices[id]; !d {
		return fmt.Errorf("Device do not exist")
	}

	delete(p.devices, id)
	return nil
}

// Control device paths.
func dataControl(deviceChan chan entities.Device, p *protocol) {
	for device := range deviceChan {

		// Creates a new device if it doesn't exist
		err := p.createDevice(device)
		if err != nil {
			log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
		}
		device = p.devices[device.ID]

		switch device.State {

		// If the device status is new, request a device registration
		case entities.KnotNew:

			device.State = entities.KnotWait
			err = p.updateDevice(device)
			if err != nil {
				log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
			} else {
				err = p.network.publisher.PublishDeviceRegister(p.userToken, &device)
				if err != nil {
					log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
				} else {
					log.WithFields(log.Fields{"knot": entities.KnotNew}).Info("send a register request")
				}
			}
		// If the device is already registered, ask for device authentication
		case entities.KnotRegistered:
			device.State = entities.KnotWait
			err = p.updateDevice(device)
			if err != nil {
				log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
			} else {
				err = p.network.publisher.PublishDeviceAuth(p.userToken, &device)
				if err != nil {
					log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
				} else {
					log.WithFields(log.Fields{"knot": entities.KnotRegistered}).Info("send a auth request")
				}
			}
		// Now the device has a token, make a new request for authentication.
		case entities.KnotAuth:
			device.State = entities.KnotWait
			err = p.updateDevice(device)
			if err != nil {
				log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
			} else {
				err = p.network.publisher.PublishDeviceUpdateConfig(p.userToken, &device)
				if err != nil {
					log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
				} else {
					log.WithFields(log.Fields{"knot": entities.KnotAuth}).Info("send the new configuration")
				}
			}
		// Send the new data that comes from the device to Knot Cloud
		case entities.KnotOk:
			err = p.checkData(device)
			if err != nil {
				log.WithFields(log.Fields{"debug": "true", "Update Device": "Error"}).Error("invalid data")
			} else {
				err = p.network.publisher.PublishDeviceData(p.userToken, &device, device.Data)
				if err != nil {
					log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
				} else {
					device.Data = nil
					err = p.updateDevice(device)
					if err != nil {
						log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
					} else {
						log.WithFields(log.Fields{"knot": entities.KnotAuth}).Info("send the new data comes from the device")
					}
				}
			}

		// Check if the device has a token, if it does, delete it, if not, resend the registration request
		case entities.KnotDelete:

			if device.Token != "" {
				err = p.deleteDevice(device.ID)
				if err != nil {
					log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
				} else {
					log.WithFields(log.Fields{"knot": entities.KnotDelete}).Info("delete a device")
				}
			} else {
				device.State = entities.KnotWait
				err := p.updateDevice(device)
				if err != nil {
					log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
				} else {
					err = p.network.publisher.PublishDeviceRegister(p.userToken, &device)
					if err != nil {
						log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
					} else {
						log.WithFields(log.Fields{"knot": entities.KnotDelete}).Info("send a register request")
					}
				}
			}

		// Just delete
		case entities.KnotForceDelete:

			err = p.deleteDevice(device.ID)
			if err != nil {
				log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
			} else {
				log.WithFields(log.Fields{"knot": entities.KnotDelete}).Info("delete a device")
			}

		// Handle errors
		case entities.KnotError:
			switch device.Error {
			// If the device is new to the chirpstack platform, but already has a registration in Knot, first the device needs to ask to unregister and then ask for a registration.
			case "thing is already registered":
				log.WithFields(log.Fields{"knot": entities.KnotError}).Error("device is registered, but does not have a token; send a unregister request")
				p.network.publisher.PublishDeviceUnregister(p.userToken, &device)

			case "thing's config not provided":
				log.WithFields(log.Fields{"knot": entities.KnotError}).Error("device is registered, but does not have a token; send a unregister request")

			default:
				log.WithFields(log.Fields{"knot": entities.KnotError}).Error("ERROR WITHOUT HANDLER" + device.Error)

			}
			device.State = entities.KnotWait
			device.Error = ""
			err := p.updateDevice(device)
			if err != nil {
				log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
			}
		case entities.KnotWait:

		}
	}
}

// Just formart the Error message
func errorFormat(device entities.Device, strError string) entities.Device {
	device.Error = strError
	device.State = entities.KnotError
	log.WithFields(log.Fields{"amqp": "knot"}).Error(strError)
	return device
}

// Handles messages coming from AMQP
func handlerKnotAMQP(msgChan <-chan network.InMsg, deviceChan chan entities.Device) {

	for message := range msgChan {

		switch message.RoutingKey {

		// Registered msg from knot
		case network.BindingKeyRegistered:
			log.WithFields(log.Fields{"amqp": "knot"}).Info("received a registration response")
			device := entities.Device{}

			receiver := network.DeviceRegisteredResponse{}

			err := json.Unmarshal([]byte(string(message.Body)), &receiver)
			if err != nil {
				log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
			}
			device.ID = receiver.ID
			device.Name = receiver.Name

			if receiver.Error != "" {
				// Alread registered
				log.WithFields(log.Fields{"amqp": "knot"}).Info("received a registration response with a error")
				deviceChan <- errorFormat(device, receiver.Error)
			} else {
				device.Token = receiver.Token
				device.State = entities.KnotRegistered
				deviceChan <- device
			}

		// Unregistered
		case network.BindingKeyUnregistered:
			log.WithFields(log.Fields{"amqp": "knot"}).Info("received a unregistration response")
			device := entities.Device{}

			receiver := network.DeviceUnregisterRequest{}

			err := json.Unmarshal([]byte(string(message.Body)), &receiver)
			if err != nil {
				log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
			} else {
				device.ID = receiver.ID
				device.State = entities.KnotDelete
				deviceChan <- device
			}

		// Receive a auth msg
		case network.ReplyToAuthMessages:
			log.WithFields(log.Fields{"amqp": "knot"}).Info("received a authentication response")
			device := entities.Device{}

			receiver := network.DeviceAuthResponse{}

			err := json.Unmarshal([]byte(string(message.Body)), &receiver)
			if err != nil {
				log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
			} else {
				device.ID = receiver.ID

				if receiver.Error != "" {
					// Alread registered
					deviceChan <- errorFormat(device, receiver.Error)
				} else {
					device.State = entities.KnotAuth
					deviceChan <- device

				}
			}
		case network.BindingKeyUpdatedConfig:
			log.WithFields(log.Fields{"amqp": "knot"}).Info("received a config update response")
			device := entities.Device{}

			receiver := network.ConfigUpdatedResponse{}

			err := json.Unmarshal([]byte(string(message.Body)), &receiver)
			if err != nil {
				log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
			} else {
				device.ID = receiver.ID
				if receiver.Error != "" {
					// Alread registered
					deviceChan <- errorFormat(device, receiver.Error)
				} else {
					device.State = entities.KnotOk
					deviceChan <- device
				}
			}
		}
	}
}
