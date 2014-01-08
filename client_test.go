package apns

import (
	"fmt"
	"log"
	"time"
)

func getPN() *PushNotification {
	pn := NewPushNotification()

	pn.DeviceToken = "af7685af756476543987af"

	payload := NewPayload()
	payload.Sound = "default"
	payload.Alert = "hi!"
	pn.AddPayload(payload)
	return pn
}

func Example() {
	mc := NewClient("gateway.sandbox.push.apple.com:2195", "certs/p1-dev-cert.pem", "certs/p1-dev-key.pem")
	go func() {
		for {
			err := mc.Run()
			log.Println(err)
		}
	}()
	go func() {
		for {
			x := <-mc.FailedNotifications()
			var msg string
			if x.AppleResponse != nil {
				msg = *x.AppleResponse
			} else {
				msg = (*x.Error).Error()
			}
			log.Println("ErrorChannel received id", x.PushNotification.Identifier, msg)
		}
	}()
	for i := 0; i < 3; i++ {
		p := getPN()
		mc.Queue(p)
		log.Println("Queued id", p.Identifier)
	}
	time.Sleep(time.Second * 60)
	fmt.Println("done")
	// Output: done
}
