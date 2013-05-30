package apns

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/rand"
	"strconv"
	"time"
)

const PUSH_COMMAND_VALUE = 1
const MAX_PAYLOAD_SIZE_BYTES = 256
const IDENTIFIER_UBOUND = 9999

// Alert is an interface here because it supports either a string
// or a dictionary, represented within by an AlertDictionary struct.
type Payload struct {
	Alert interface{} `json:"alert,omitempty"`
	Badge int         `json:"badge,omitempty"`
	Sound string      `json:"sound,omitempty"`
}

func NewPayload() *Payload {
	return new(Payload)
}

// From the APN docs: "Use the ... alert dictionary in general only if you absolutely need to."
type AlertDictionary struct {
	Body         string   `json:"body,omitempty"`
	ActionLocKey string   `json:"action-loc-key,omitempty"`
	LocKey       string   `json:"loc-key,omitempty"`
	LocArgs      []string `json:"loc-args,omitempty"`
	LaunchImage  string   `json:"launch-image,omitempty"`
}

func NewAlertDictionary() *AlertDictionary {
	return new(AlertDictionary)
}

// The length fields are computed in ToBytes() and aren't represented here.
type PushNotification struct {
	Identifier  int32
	Expiry      uint32
	DeviceToken string
	payload     map[string]interface{}
}

func NewPushNotification() (pn *PushNotification) {
	pn = new(PushNotification)
	pn.payload = make(map[string]interface{})
	pn.Identifier = rand.New(rand.NewSource(time.Now().UnixNano())).Int31n(IDENTIFIER_UBOUND)
	return
}

func (this *PushNotification) AddPayload(p *Payload) {
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
	if len(payload) > MAX_PAYLOAD_SIZE_BYTES {
		return nil, errors.New("payload is larger than the " + strconv.Itoa(MAX_PAYLOAD_SIZE_BYTES) + " byte limit")
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
