package apns

import (
	"testing"
	"time"
)

func getPN() *PushNotification {
	pn := NewPushNotification()

	pn.DeviceToken = "af7685af756476543987af"

	payLoad := NewPayload()
	payLoad.Sound = "default"
	pn.AddPayload(payLoad)
	return pn
}

func TestA(t *testing.T) {
	mc := NewMultiClient("gateway.sandbox.push.apple.com:2195", "certs/p1-dev-cert.pem", "certs/p1-dev-key.pem")
	go mc.Run()

	mc.Queue(getPN())
	mc.Queue(getPN())
	mc.Queue(getPN())
	//mc.Queue(getPN())
	time.Sleep(time.Second * 20)
}
