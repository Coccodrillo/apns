// apns.go
//
// Documentation related to the JSON alert message:
// http://developer.apple.com/library/mac/#documentation/NetworkingInternet/Conceptual/RemoteNotificationsPG/Chapters/ApplePushService.html
//
// Documentation related to the binary protocol:
// http://developer.apple.com/library/ios/#documentation/NetworkingInternet/Conceptual/RemoteNotificationsPG/Chapters/CommunicatingWIthAPS.html
package apns

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
)

// The first byte of the push notification payload is the command value,
// which at this time is fixed to 1 by Apple.
const PUSH_COMMAND_VALUE = 1

// The total length of the payload cannot exceed this amount.
const MAX_PAYLOAD_SIZE_BYTES = 256

// The Envelope is the overall wrapper for the various push notification fields.
// It contains a Message, which may either contain a BasicAlert (which is a simple string
// message) or a DictionaryAlert (which contains localization fields). Apple recommends using
// the basic alert structure if at all possible.
//
// You can also set CustomProperties ( "acme": "foo" ) if desired. See the documentation
// listed at the top of this file for more information on how to best use APNs.
type Envelope struct {
	DeviceToken      string                 `json:"-"`
	Identifier       int32                  `json:"-"`
	Expiry           uint32                 `json:"-"`
	CustomProperties map[string]interface{} `json:"-"`
}

type Message struct {
	BasicAlert      string           `json:"alert,omitempty"`
	DictionaryAlert *DictionaryAlert `json:"alert,omitempty"`
	Badge           int              `json:"badge,omitempty"`
	Sound           string           `json:"sound,omitempty"`
}

type DictionaryAlert struct {
	Body         string   `json:"body,omitempty"`
	ActionLocKey string   `json:"action-loc-key,omitempty"`
	LocKey       string   `json:"loc-key,omitempty"`
	LocArgs      []string `json:"loc-args,omitempty"`
	LaunchImage  string   `json:"launch-image,omitempty"`
}

// Inserts the message into the envelope.
func (this *Envelope) SetMessage(m *Message) {
	this.Set("aps", m)
}

// The push notification requests support arbitrary custom metadata.
// This method requires a string key and any type for the value.
func (this *Envelope) Set(key string, value interface{}) {
	if this.CustomProperties == nil {
		this.CustomProperties = make(map[string]interface{})
	}
	this.CustomProperties[key] = value
}

// This method looks up a string key and returns the value, or nil.
func (this *Envelope) Get(key string) (value interface{}) {
	if this.CustomProperties == nil {
		value = nil
	} else {
		value = this.CustomProperties[key]
	}
	return
}

// Returns the JSON structure as a byte array.
func (this *Envelope) ToJSON() (j []byte, err error) {
	j, err = json.Marshal(this.CustomProperties)
	return
}

// Returns the JSON structure as a string.
func (this *Envelope) ToString() (s string, err error) {
	j, err := this.ToJSON()
	if err != nil {
		return "", err
	}
	s = string(j)
	return
}

// Returns a byte array of the complete Envelope struct. This array
// is what should be transmitted to the APN Service.
func (this *Envelope) ToBytes() (b []byte, err error) {
	token, err := hex.DecodeString(this.DeviceToken)
	if err != nil {
		return nil, err
	}
	payload, err := this.ToJSON()
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
	b = buffer.Bytes()

	return b, nil
}
