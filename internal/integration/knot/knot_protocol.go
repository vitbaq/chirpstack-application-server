package knot

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/brocaar/chirpstack-application-server/internal/config"
	"github.com/brocaar/chirpstack-application-server/internal/integration/knot/entities"
	"github.com/brocaar/chirpstack-application-server/internal/integration/knot/network"
	"github.com/brocaar/chirpstack-application-server/internal/integration/knot/values"
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
	sendKnotRequests(deviceChan chan entities.Device, oldState, curState string, device entities.Device)
	publishData(device entities.Device)
	generateID(device entities.Device) entities.Device
	validateKnotDevice(device entities.Device) entities.Device
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

	go registerDevices(p.devices, deviceChan)
	go handlerKnotAMQP(msgChan, deviceChan)
	go knotStateMachineHandler(deviceChan, p)

	return p, nil
}

//Test Device
func registerDevices(devices map[string]entities.Device, deviceChan chan entities.Device) {
	for _, device := range devices {
		device.State = values.KNoTNew
		deviceChan <- device
	}
}

//Map the knot devices from the config file existsarray
func (p *protocol) mapDevices(devices []entities.Device) {
	p.devices = make(map[string]entities.Device)
	for _, device := range devices {
		if device.DevEUI != "" {
			p.devices[device.DevEUI] = device
		}
	}
}

//Load the device's configuration
func (p *protocol) LoadDeviceOldContext() {
	if ok := checkFile(values.DeviceContex); ok {
		p.readDeviceFile(values.DeviceContex)
		log.WithFields(log.Fields{"integration": "ConfigFile"}).Info("exists")
	} else {
		p.writeDeviceFile(values.DeviceContex)
		log.WithFields(log.Fields{"integration": "ConfigFile"}).Info("create")
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
		if curDevice, ok := p.devices[oldDevice.DevEUI]; ok {
			curDevice.Token = oldDevice.Token
			curDevice.ID = oldDevice.ID
			p.devices[oldDevice.DevEUI] = curDevice
		}
	}
}

// Write a file
func (p *protocol) writeDeviceFile(pathFile string) {
	devices := p.devices

	data, err := yaml.Marshal(&devices)
	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile(pathFile, data, values.RW_R__R__)
	if err != nil {
		log.Fatal(err)
	}
}

// Check for data to be updated
func (p *protocol) checkData(device entities.Device) error {

	if device.Data == nil {
		return nil
	}

	sliceSize := len(device.Data)
	j := 0
	for i, data := range device.Data {
		if data.Value == "" {
			return fmt.Errorf("invalid sensor value")
		} else if data.Timestamp == "" {
			return fmt.Errorf("invalid sensor timestamp")
		}

		j = i + 1
		for j < sliceSize {
			if data.SensorID == device.Data[j].SensorID {
				return fmt.Errorf("repeated sensor id")
			}
			j++
		}
	}

	return nil
}

func isInvalidValueType(valueType int) bool {
	minValueTypeAllow := 1
	maxValueTypeAllow := 5
	return valueType < minValueTypeAllow || valueType > maxValueTypeAllow
}

// Check for device configuration
func (p *protocol) checkDeviceConfiguration(device entities.Device) error {
	sliceSize := len(device.Config)
	j := 0

	if device.Config == nil {
		return fmt.Errorf("sensor has no configuration")
	}

	// Check if the ids are correct, no repetition
	for i, config := range device.Config {
		if isInvalidValueType(config.Schema.ValueType) {
			return fmt.Errorf("invalid sensor id")
		}

		j = i + 1
		for j < sliceSize {
			if config.SensorID == device.Config[j].SensorID {
				return fmt.Errorf("repeated sensor id")
			}
			j++
		}
	}

	return nil

}

// Update the knot device information on map
func (p *protocol) updateDevice(device entities.Device) {

	receiver := p.devices[device.DevEUI]

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

	p.devices[device.DevEUI] = receiver

	p.writeDeviceFile(values.DeviceContex)

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

		device.State = values.KNoTNew

		p.devices[device.DevEUI] = device

		return nil
	}
}

// Check if the device exists
func (p *protocol) deviceExists(device entities.Device) bool {

	if _, checkDevice := p.devices[device.DevEUI]; checkDevice {

		return true
	}
	log.WithFields(log.Fields{"knot": values.KNoTError}).Error("Device not find" + device.Error)
	return false
}

// Delete the knot device from map
func (p *protocol) deleteDevice(name string) error {
	if _, d := p.devices[name]; !d {
		return fmt.Errorf("device do not exist")
	}

	delete(p.devices, name)
	return nil
}

//init the timeout couter
func initTimeout(deviceChan chan entities.Device, device entities.Device) {
	go func(deviceChan chan entities.Device, device entities.Device) {
		time.Sleep(20 * time.Second)
		device.Error = values.ErrorTimeOut
		deviceChan <- device
	}(deviceChan, device)
}

//Find device name by Id
func (p *protocol) findDeviceById(id string) string {
	for _, device := range p.devices {
		if device.ID == id {
			return device.DevEUI
		}
	}
	return ""
}

//check if response was received by comparing previous state with the new one
func (p *protocol) checkTimeout(device entities.Device) entities.Device {
	curDevice := p.devices[device.DevEUI]
	if device.Error == values.ErrorTimeOut {
		if verifyIfStateChange(device, curDevice) {
			log.WithFields(log.Fields{"dev_name": device.Name}).Error("TimeOut")
		} else {
			device.State = values.KNoTOff
			return device
		}
	}
	return device
}

// Verify is the current device still in the same state as when the timeout was started
func verifyIfStateChange(deviceBeforeTimeout, currentDevice entities.Device) bool {
	return (deviceBeforeTimeout.State == values.KNoTNew && currentDevice.State == values.KNoTWaitReg) ||
		(deviceBeforeTimeout.State == values.KNoTRegistered && currentDevice.State == values.KNoTWaitAuth) ||
		(deviceBeforeTimeout.State == values.KNoTAuth && currentDevice.State == values.KNoTWaitConfig) ||
		(deviceBeforeTimeout.State == values.KNoTAlreadyReg && currentDevice.State == values.KNoTWaitUnreg)
}

// Send request to amqp knot
func (p *protocol) sendKnotRequests(deviceChan chan entities.Device, oldState, curState string, device entities.Device) {
	device.State = oldState
	initTimeout(deviceChan, device)
	var err error
	switch device.State {
	case values.KNoTNew:
		log.WithFields(log.Fields{"knot": values.KNoTNew}).Info("send a register request")
		err = p.network.publisher.PublishDeviceRegister(p.userToken, &device)
	case values.KNoTRegistered:
		log.WithFields(log.Fields{"knot": values.KNoTRegistered}).Info("send a auth request")
		err = p.network.publisher.PublishDeviceAuth(p.userToken, &device)
	case values.KNoTAuth:
		log.WithFields(log.Fields{"knot": values.KNoTAuth}).Info("send the configuration request")
		err = p.network.publisher.PublishDeviceUpdateConfig(p.userToken, &device)
	}
	if err != nil {
		log.WithFields(log.Fields{"knot": values.KNoTError}).Error(err)
	} else {
		device.State = curState
		p.updateDevice(device)
	}
}

// Generated a new Device ID
func idGenerator() (string, error) {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

// Create a new device ID
func (p *protocol) generateID(device entities.Device) entities.Device {
	delete(p.devices, device.DevEUI)
	var err error
	device.ID, err = idGenerator()
	if err != nil {
		device.State = values.KNoTOff
		return device
	}

	device.Token = ""
	device.State = values.KNoTNew
	p.devices[device.DevEUI] = device

	return device
}

//Validate device
func (p *protocol) validateKnotDevice(device entities.Device) entities.Device {

	if device.DevEUI == "" && device.ID == "" {
		device.State = values.KNoTOff
		return device
	} else if device.DevEUI == "" && device.ID != "" {
		device.DevEUI = p.findDeviceById(device.ID)
	}

	if p.deviceExists(device) {
		log.Info("received : " + device.DevEUI)
		device = p.checkTimeout(device)
		if device.State != values.KNoTOff && device.Error != values.ErrorTimeOut {
			p.updateDevice(device)
			device = p.devices[device.DevEUI]

			if device.ID == "" || device.Token == "" || device.Token == "invalid" {
				device = p.generateID(device)
				p.updateDevice(device)
			} else if device.State == values.KNoTNew && device.Token != "" {
				device.State = values.KNoTRegistered
			}

		} else if device.Error == values.ErrorTimeOut {
			device.Error = values.NoError
		}
	} else {
		device.State = values.KNoTOff
	}

	return device
}

// Control device paths.
func knotStateMachineHandler(deviceChan chan entities.Device, p *protocol) {
	for device := range deviceChan {

		device = p.validateKnotDevice(device)

		switch device.State {

		// If the device status is new, request a device registration
		case values.KNoTNew:
			p.sendKnotRequests(deviceChan, device.State, values.KNoTWaitReg, device)

		// If the device is already registered, ask for device authentication
		case values.KNoTRegistered:
			p.sendKnotRequests(deviceChan, device.State, values.KNoTWaitAuth, device)

		// Now the device has a token, make a new request for authentication.
		case values.KNoTAuth:
			p.sendKnotRequests(deviceChan, device.State, values.KNoTWaitConfig, device)

		// Send the new data that comes from the device to Knot Cloud
		case values.KNoTOk:
			p.publishData(device)

		// Handle errors
		case values.KNoTError:
			log.WithFields(log.Fields{"knot": values.KNoTError}).Error("ERROR WITHOUT HANDLER: " + device.Error)
			device.State = values.KNoTError
			device.Error = values.UnhandledError
			p.updateDevice(device)

		case values.KNoTOff:

		}
	}
}

//Post each message at a time
func (p *protocol) publishData(device entities.Device) {
	err := p.checkData(device)
	if err != nil {
		log.WithFields(log.Fields{"knot": "error"}).Error(err)
	} else {
		// take all data from the device
		data := device.Data
		device.Data = nil
		for _, register := range data {
			//send each data at a time
			device.Data = append(device.Data, register)
			err := p.network.publisher.PublishDeviceData(p.userToken, &device, device.Data)
			if err != nil {
				log.WithFields(log.Fields{"knot": "error"}).Error(err)
			} else {
				log.WithFields(log.Fields{"knot": values.KNoTAuth}).Info("send the new data comes from the device")
			}
			device.Data = nil
		}
		p.updateDevice(device)
	}
}

func amqpReceiver(message network.InMsg, nextStateIfNotError, msg string) entities.Device {
	log.WithFields(log.Fields{"amqp": "knot"}).Info(msg)
	device := entities.Device{}
	receiver := network.DeviceMessage{}
	err := json.Unmarshal([]byte(string(message.Body)), &receiver)
	if err != nil {
		log.WithFields(log.Fields{"knot": "error"}).Error(err)
		device.State = values.KNoTOff
		return device
	}

	if network.BindingKeyRegistered == message.RoutingKey && receiver.Token != "" {
		device.Token = receiver.Token
	}

	device.DevEUI = ""
	device.ID = receiver.ID
	device = verifyKnotErrors(device, receiver, nextStateIfNotError)

	if receiver.Name != "" {
		device.Name = receiver.Name
	}

	return device

}

func verifyKnotErrors(device entities.Device, receiver network.DeviceMessage, nextStateIfNotError string) entities.Device {

	if receiver.Error == values.ErrorRegistered {
		device.Token = "invalid"
		device.State = values.KNoTNew
		return device
	} else if receiver.Error == values.ErrorNoConfig {
		log.WithFields(log.Fields{"amqp": "knot"}).Info("device has no configuration")
		device.State = values.KNoTError
		return device
	} else if receiver.Error == values.ErrorFailMetadata {
		log.WithFields(log.Fields{"amqp": "knot"}).Info("fail validation metadata")
		device.Token = "invalid"
		device.State = values.KNoTNew
		return device
	} else if receiver.Error != values.NoError {
		log.WithFields(log.Fields{"amqp": "knot"}).Info("KNoT return the error: " + receiver.Error)
		device.State = values.KNoTError
		return device
	} else {
		device.State = nextStateIfNotError
		return device
	}
}

// Handles messages coming from AMQP
func handlerKnotAMQP(msgChan <-chan network.InMsg, deviceChan chan entities.Device) {

	for message := range msgChan {

		switch message.RoutingKey {
		case network.BindingKeyRegistered:
			deviceChan <- amqpReceiver(message, values.KNoTRegistered, "received a registration response")
		case network.BindingKeyUnregistered:
			deviceChan <- amqpReceiver(message, values.KNoTNew, "received a unregistration response")
		case network.ReplyToAuthMessages:
			deviceChan <- amqpReceiver(message, values.KNoTAuth, "received a authentication response")
		case network.BindingKeyUpdatedConfig:
			deviceChan <- amqpReceiver(message, values.KNoTOk, "received a config update response")
		}
	}
}
