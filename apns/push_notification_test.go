package apns

import (
	"testing"
)

func TestBasicAlert(t *testing.T) {
	message := new(Message)
	message.Alert = "You have mail!"

	envelope := new(Envelope)
	envelope.SetMessage(message)

	bytes, _ := envelope.ToBytes()
	if len(bytes) != 47 {
		t.Error("expected 47 bytes; got", len(bytes))
	}

	json, _ := envelope.PayloadJSON()
	if len(json) != 34 {
		t.Error("expected 34 bytes; got", len(json))
	}
}

func TestDictionaryAlert(t *testing.T) {
	args := make([]string, 1)
	args[0] = "localized args"

	dict := new(DictionaryAlert)
	dict.Body = "Complex Message"
	dict.ActionLocKey = "Play a Game!"
	dict.LocKey = "localized key"
	dict.LocArgs = args
	dict.LaunchImage = "image.jpg"

	message := new(Message)
	message.Badge = 42
	message.Sound = "bingbong.aiff"
	message.Alert = dict

	envelope := new(Envelope)
	envelope.SetMessage(message)

	bytes, _ := envelope.ToBytes()
	if len(bytes) != 207 {
		t.Error("expected 207 bytes; got", len(bytes))
	}

	json, _ := envelope.PayloadJSON()
	if len(json) != 194 {
		t.Error("expected 194 bytes; got", len(bytes))
	}
}

func TestCustomParameters(t *testing.T) {
	message := new(Message)
	message.Alert = "You have mail!"

	envelope := new(Envelope)
	envelope.SetMessage(message)
	envelope.Set("Doctor", "Who")

	bytes, _ := envelope.ToBytes()
	if len(bytes) != 62 {
		t.Error("expected 62 bytes; got", len(bytes))
	}

	json, _ := envelope.PayloadJSON()
	if len(json) != 49 {
		t.Error("expected 49 bytes; got", len(bytes))
	}

	if envelope.Get("Doctor") != "Who" {
		t.Error("unable to set a custom property")
	}

	if envelope.Get("Not Set") != nil {
		t.Error("expected a missing key to return nil")
	}
}
