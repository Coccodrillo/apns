package apns

import (
	"testing"
)

// Create a new Payload that specifies simple text,
// a badge counter, and a custom notification sound.
func mockPayload() (payload *Payload) {
	payload = NewPayload()
	payload.Alert = "You have mail!"
	badge := 42
	payload.Badge = &badge
	payload.Sound = "bingbong.aiff"
	return
}

// Create a new AlertDictionary. Apple recommends you not use
// the more complex alert style unless absolutely necessary.
func mockAlertDictionary() (dict *AlertDictionary) {
	args := make([]string, 1)
	args[0] = "localized args"

	dict = NewAlertDictionary()
	dict.Body = "Complex Message"
	dict.ActionLocKey = "Play a Game!"
	dict.LocKey = "localized key"
	dict.LocArgs = args
	dict.LaunchImage = "image.jpg"
	return
}

func TestBasicAlert(t *testing.T) {
	payload := mockPayload()
	pn := NewPushNotification()

	pn.AddPayload(payload)

	bytes, _ := pn.ToBytes()
	json, _ := pn.PayloadJSON()
	if len(bytes) != 82 {
		t.Error("expected 82 bytes; got", len(bytes))
	}
	if len(json) != 69 {
		t.Error("expected 69 bytes; got", len(json))
	}
}

func TestAlertDictionary(t *testing.T) {
	dict := mockAlertDictionary()
	payload := mockPayload()
	payload.Alert = dict

	pn := NewPushNotification()
	pn.AddPayload(payload)

	bytes, _ := pn.ToBytes()
	json, _ := pn.PayloadJSON()
	if len(bytes) != 207 {
		t.Error("expected 207 bytes; got", len(bytes))
	}
	if len(json) != 194 {
		t.Error("expected 194 bytes; got", len(bytes))
	}
}

func TestCustomParameters(t *testing.T) {
	payload := mockPayload()
	pn := NewPushNotification()

	pn.AddPayload(payload)
	pn.Set("foo", "bar")

	if pn.Get("foo") != "bar" {
		t.Error("unable to set a custom property")
	}
	if pn.Get("not_set") != nil {
		t.Error("expected a missing key to return nil")
	}

	bytes, _ := pn.ToBytes()
	json, _ := pn.PayloadJSON()
	if len(bytes) != 94 {
		t.Error("expected 94 bytes; got", len(bytes))
	}
	if len(json) != 81 {
		t.Error("expected 81 bytes; got", len(json))
	}
}

func TestZeroBadge(t *testing.T) {
	payload := NewPayload()
	badge := 0
	payload.Badge = &badge
	pn := NewPushNotification()
	pn.AddPayload(payload)

	json, err := pn.PayloadString()
	if err != nil {
		t.Error(err)
	}
	expected := `{"aps":{"badge":0}}`
	if json != expected {
		t.Errorf("Expected json %v but got %v", expected, json)
	}
}

func TestNilBadge(t *testing.T) {
	payload := NewPayload()
	pn := NewPushNotification()
	pn.AddPayload(payload)

	json, err := pn.PayloadString()
	if err != nil {
		t.Error(err)
	}
	expected := `{"aps":{}}`
	if json != expected {
		t.Errorf("Expected json %v but got %v", expected, json)
	}
}
