package knot

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/brocaar/chirpstack-application-server/internal/config"
	"github.com/brocaar/chirpstack-application-server/internal/integration/knot/entities"
	"github.com/brocaar/chirpstack-application-server/internal/integration/knot/network"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

// Protocol interface provides methods to handle KNoT Protocol
type Protocol interface {
	Close() error
	createDevice(device entities.Device) error
	deleteDevice(id string) error
	updateDevice(device entities.Device)
	checkData(device entities.Device) error
	checkDeviceConfiguration(device entities.Device) error
	deviceExists(device entities.Device) bool
	readDeviceFile(name string)
	LoadDeviceOldContext()
	writeDeviceFile(name string)
	checkTimeout(device entities.Device) entities.Device
	findDeviceById(id string) string
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
		log.WithFields(log.Fields{"integration": "knot"}).Info("New connection to knot-cloud")
	}

	p.network.publisher = network.NewMsgPublisher(p.network.amqp)
	p.network.subscriber = network.NewMsgSubscriber(p.network.amqp)

	if err = p.network.subscriber.SubscribeToKNoTMessages(msgChan); err != nil {
		log.WithFields(log.Fields{"integration": "knot"}).Error("Error to subscribe")
		return p, err
	}

	p.mapDevices(conf.Devices)
	p.LoadDeviceOldContext()
	// devices, err := LoadDeviceOldContext()
	// if err != nil {
	// 	log.WithFields(log.Fields{"integration": "knot"}).Error(err)
	// }
	go registerDevices(p.devices, deviceChan)
	go handlerKnotAMQP(msgChan, deviceChan)
	go dataControl(deviceChan, p)

	return p, nil
}

//Teste Device
func registerDevices(devices map[string]entities.Device, deviceChan chan entities.Device) {
	for _, device := range devices {
		device.State = entities.KnotNew
		deviceChan <- device
	}
}

//Map the knot devices from the config file array
func (p *protocol) mapDevices(devices []entities.Device) {
	p.devices = make(map[string]entities.Device)
	for _, device := range devices {
		p.devices[device.ID] = device
	}
}

//Load the device's configuration
func (p *protocol) LoadDeviceOldContext() {
	if ok := checkFile("/tmp/device/deviceConfig.yaml"); ok {
		p.readDeviceFile("/tmp/device/deviceConfig.yaml")
		log.WithFields(log.Fields{"integration": "ConfigFile"}).Info("existe")
	} else {
		p.writeDeviceFile("/tmp/device/deviceConfig.yaml")
		log.WithFields(log.Fields{"integration": "ConfigFile"}).Info("Criando")
	}
}

//check if the file exists
func checkFile(name string) bool {
	if _, err := os.Stat(name); err == nil {
		// file exists
		return true

	} else if errors.Is(err, os.ErrNotExist) {
		// file does *not* exist
		return false
	}
	return false
}

//Read the device config file
func (p *protocol) readDeviceFile(name string) {
	var config map[string]entities.Device

	yamlBytes, err := ioutil.ReadFile(name)
	if err != nil {
		log.Fatal(err)
	}

	unmarshalErr := yaml.Unmarshal(yamlBytes, &config)
	if unmarshalErr != nil {
		log.Fatal(unmarshalErr)
	}
	for _, oldDevice := range config {
		if curDevice, ok := p.devices[oldDevice.ID]; ok {
			curDevice.Token = oldDevice.Token
			p.devices[oldDevice.ID] = curDevice
		}
	}
}

// Write a file
func (p *protocol) writeDeviceFile(name string) {
	data, err := yaml.Marshal(p.devices)
	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile(name, data, 0600)
	if err != nil {
		log.Fatal(err)
	}
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
func (p *protocol) updateDevice(device entities.Device) {

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

	p.writeDeviceFile("/tmp/device/deviceConfig.yaml")

}

// Close closes the protocol.
func (p *protocol) Close() error {
	p.network.amqp.Stop()
	return nil
}

// Create a new knot device
func (p *protocol) createDevice(device entities.Device) error {

	if device.State != "" {
		return fmt.Errorf("device cannot be created, unknown source")
	} else {
		log.WithFields(log.Fields{"dev_name": device.Name}).Info("Device created")

		device.State = entities.KnotNew

		p.devices[device.ID] = device

		return nil
	}
}

// Check if the device exists
func (p *protocol) deviceExists(device entities.Device) bool {

	if _, checkDevice := p.devices[device.ID]; checkDevice {

		return true
	}
	return false
}

// Delete the knot device from map
func (p *protocol) deleteDevice(name string) error {
	if _, d := p.devices[name]; !d {
		return fmt.Errorf("Device do not exist")
	}

	delete(p.devices, name)
	return nil
}

//init the timeout couter
func initTimeout(deviceChan chan entities.Device, device entities.Device) {
	go func(deviceChan chan entities.Device, device entities.Device) {
		time.Sleep(20 * time.Second)
		device.Error = "timeOut"
		deviceChan <- device
	}(deviceChan, device)
}

//Find device name by Id
func (p *protocol) findDeviceById(id string) string {
	for _, device := range p.devices {
		if device.ID == id {
			return device.Name
		}
	}
	return ""
}

//check if response was received by comparing previous state with the new one
func (p *protocol) checkTimeout(device entities.Device) entities.Device {
	curDevice := p.devices[device.ID]
	if device.Error == "timeOut" {
		if (device.State == entities.KnotNew && curDevice.State == entities.KnotWaitReg) ||
			(device.State == entities.KnotRegistered && curDevice.State == entities.KnotWaitAuth) ||
			(device.State == entities.KnotAuth && curDevice.State == entities.KnotWaitConfig) ||
			(device.State == entities.KnotAlreadyReg && curDevice.State == entities.KnotWaitUnreg) {
			log.WithFields(log.Fields{"dev_name": device.Name}).Error("TimeOut")
		} else {
			device.State = entities.KnotOff
			return device
		}
	}
	return device
}

// Control device paths.
func dataControl(deviceChan chan entities.Device, p *protocol) {
	for device := range deviceChan {

		if p.deviceExists(device) {
			device = p.checkTimeout(device)
			if device.State != entities.KnotOff && device.Error != "timeOut" {

				p.updateDevice(device)
				device = p.devices[device.ID]

				if device.Name == "" {
					log.Fatalln("Device has no name")
				} else if device.State == entities.KnotNew {
					if device.Token != "" {
						device.State = entities.KnotRegistered
					}
				}
			} else if device.Error == "timeOut" {
				device.Error = ""
			}
			log.WithFields(log.Fields{" knot": "STATE "}).Info(device.State)

			switch device.State {

			// If the device status is new, request a device registration
			case entities.KnotNew:

				initTimeout(deviceChan, device)
				device.State = entities.KnotWaitReg
				p.updateDevice(device)
				err := p.network.publisher.PublishDeviceRegister(p.userToken, &device)
				if err != nil {
					log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
				} else {
					log.WithFields(log.Fields{"knot": entities.KnotNew}).Info("send a register request")
				}
			// If the device is already registered, ask for device authentication
			case entities.KnotRegistered:
				initTimeout(deviceChan, device)
				device.State = entities.KnotWaitAuth
				p.updateDevice(device)
				err := p.network.publisher.PublishDeviceAuth(p.userToken, &device)
				if err != nil {
					log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
				} else {
					log.WithFields(log.Fields{"knot": entities.KnotRegistered}).Info("send a auth request")
				}
			// Now the device has a token, make a new request for authentication.
			case entities.KnotAuth:
				initTimeout(deviceChan, device)
				device.State = entities.KnotWaitConfig
				p.updateDevice(device)
				err := p.network.publisher.PublishDeviceUpdateConfig(p.userToken, &device)
				if err != nil {
					log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
				} else {
					log.WithFields(log.Fields{"knot": entities.KnotAuth}).Info("send the new configuration")
				}

			// Send the new data that comes from the device to Knot Cloud
			case entities.KnotOk:
				err := p.checkData(device)
				if err != nil {
					log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
				} else {
					err = p.network.publisher.PublishDeviceData(p.userToken, &device, device.Data)
					if err != nil {
						log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
					} else {
						device.Data = nil
						p.updateDevice(device)
						log.WithFields(log.Fields{"knot": entities.KnotAuth}).Info("send the new data comes from the device")
					}
				}

			// Check if the device has a token, go to authentication, if not,  unregister
			case entities.KnotAlreadyReg:

				if device.Token == "" {
					initTimeout(deviceChan, device)
					device.State = entities.KnotWaitUnreg
					p.updateDevice(device)
					err := p.network.publisher.PublishDeviceUnregister(p.userToken, &device)
					if err != nil {
						log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
					} else {
						log.WithFields(log.Fields{"knot": entities.KnotAuth}).Info("send a unregister request")
					}
				} else {
					device.State = entities.KnotRegistered
					initTimeout(deviceChan, device)
					device.State = entities.KnotWaitAuth
					p.updateDevice(device)
					err := p.network.publisher.PublishDeviceAuth(p.userToken, &device)
					if err != nil {
						log.WithFields(log.Fields{"knot": entities.KnotError}).Error(err)
					} else {
						log.WithFields(log.Fields{"knot": entities.KnotRegistered}).Info("send a auth request")
					}
				}

			// Handle errors
			case entities.KnotError:
				switch device.Error {
				// If the device is new to the chirpstack platform, but already has a registration in Knot, first the device needs to ask to unregister and then ask for a registration.
				case "thing's config not provided":
					log.WithFields(log.Fields{"knot": entities.KnotError}).Error("device is registered, but does not have a token; send a unregister request")

				default:
					log.WithFields(log.Fields{"knot": entities.KnotError}).Error("ERROR WITHOUT HANDLER" + device.Error)

				}
				device.State = entities.KnotOff
				device.Error = ""
				p.updateDevice(device)
			case entities.KnotOff:

			}
		} else {
			log.WithFields(log.Fields{"knot": entities.KnotError}).Error("Device not find" + device.Error)

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

			if receiver.Error == "thing is already registered" {
				device.State = entities.KnotAlreadyReg
				deviceChan <- device
			} else if receiver.Error != "" {
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
				device.State = entities.KnotNew
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
