package apns

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"sync/atomic"
)

// Push commands always start with command value 1.
const PUSH_COMMAND_VALUE = 1

// Your total notification payload cannot exceed 256 bytes for IOS7 and earlier, 2kb IOS8 and later.
const MAX_PAYLOAD_SIZE_BEFORE_IOS8_BYTES = 256
const MAX_PAYLOAD_SIZE_BYTES = 2048

var lastUsedNotificationIdentifier uint32

// Alert is an interface here because it supports either a string
// or a dictionary, represented within by an AlertDictionary struct.
type Payload struct {
	Alert            interface{} `json:"alert,omitempty"`
	Badge            *int        `json:"badge,omitempty"`
	Sound            string      `json:"sound,omitempty"`
	ContentAvailable *int        `json:"content-available,omitempty"`
	Category         string      `json:"category,omitempty"`
}

// Constructor.
func NewPayload() *Payload {
	return new(Payload)
}

// From the APN docs: "Use the ... alert dictionary in general only if you absolutely need to."
// The AlertDictionary is suitable for specific localization needs.
// Remember that title can be set only on IOS8.2 or greater.
type AlertDictionary struct {
	Title        string   `json:"title,omitempty"`
	Body         string   `json:"body,omitempty"`
	ActionLocKey string   `json:"action-loc-key,omitempty"`
	LocKey       string   `json:"loc-key,omitempty"`
	LocArgs      []string `json:"loc-args,omitempty"`
	LaunchImage  string   `json:"launch-image,omitempty"`
}

// Constructor.
func NewAlertDictionary() *AlertDictionary {
	return new(AlertDictionary)
}

// The PushNotification is the wrapper for the Payload.
// The length fields are computed in ToBytes() and aren't represented here.
type PushNotification struct {
	Identifier     uint32
	Expiry         uint32
	DeviceToken    string
	payload        map[string]interface{}
	maxPayloadSize int
}

// Constructor. Also initializes the pseudo-random identifier.
func NewPushNotification(isIOS7OrEarlier bool) (pn *PushNotification) {
	pn = new(PushNotification)
	pn.payload = make(map[string]interface{})
	pn.Identifier = atomic.AddUint32(&lastUsedNotificationIdentifier, 1)
	pn.maxPayloadSize = MAX_PAYLOAD_SIZE_BYTES
	if isIOS7OrEarlier {
		pn.maxPayloadSize = MAX_PAYLOAD_SIZE_BEFORE_IOS8_BYTES
	}
	return
}

func (this *PushNotification) AddPayload(p interface{}) {
	this.Set("aps", p)
}

func (this *PushNotification) Get(key string) interface{} {
	return this.payload[key]
}

func (this *PushNotification) Set(key string, value interface{}) {
	this.payload[key] = value
}

func (this *PushNotification) PayloadJSON() ([]byte, error) {
	return json.Marshal(this.payload)
}

func (this *PushNotification) PayloadString() (string, error) {
	j, err := this.PayloadJSON()
	return string(j), err
}

func (this *PushNotification) ExceededMaxPayload() (exceeded bool, extraLength int, err error) {
	exceeded = false
	payload, err := this.PayloadJSON()
	if err != nil {
		return
	}
	length := len(payload)
	if length > this.maxPayloadSize {
		exceeded = true
		extraLength = length - this.maxPayloadSize
	}
	return
}

// Returns a byte array of the complete PushNotification struct. This array
// is what should be transmitted to the APN Service.
func (this *PushNotification) ToBytes() ([]byte, error) {
	token, err := hex.DecodeString(this.DeviceToken)
	if err != nil {
		return nil, err
	}
	payload, err := this.PayloadJSON()
	if err != nil {
		return nil, err
	}

	if len(payload) > this.maxPayloadSize {
		return nil, errors.New("payload is larger than the " + strconv.Itoa(this.maxPayloadSize) + " byte limit")
	}

	buffer := bytes.NewBuffer([]byte{})
	binary.Write(buffer, binary.BigEndian, uint8(PUSH_COMMAND_VALUE))
	binary.Write(buffer, binary.BigEndian, uint32(this.Identifier))
	binary.Write(buffer, binary.BigEndian, uint32(this.Expiry))
	binary.Write(buffer, binary.BigEndian, uint16(len(token)))
	binary.Write(buffer, binary.BigEndian, token)
	binary.Write(buffer, binary.BigEndian, uint16(len(payload)))
	binary.Write(buffer, binary.BigEndian, payload)
	return buffer.Bytes(), nil
}
