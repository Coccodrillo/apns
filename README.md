# apns

Utilities for Apple Push Notification and Feedback Services.This package is still a work-in-progress.

Currently the low-level primitives for assembling JSON and binary payloads are working. Next will be the TCP client for reaching out to the APNs Service and dealing with responses. After that will be the feedback service to check for invalidated tokens.

Ideally, the common use case of sending a simple text update will be a two-line call (set up a client, send the text alert). You shouldn't need to deal with the primitives unless you so desire.

## Installation

`go get github.com/anachronistic/apns`

## Documentation

- [Information on the APN JSON payloads](http://developer.apple.com/library/mac/#documentation/NetworkingInternet/Conceptual/RemoteNotificationsPG/Chapters/ApplePushService.html)
- [Information on the APN binary protocols](http://developer.apple.com/library/ios/#documentation/NetworkingInternet/Conceptual/RemoteNotificationsPG/Chapters/CommunicatingWIthAPS.html)

## Usage

### Creating envelopes and payloads manually
```go
package main

import (
  "fmt"
  apns "github.com/anachronistic/apns"
)

func main() {
  payload := new(apns.Payload)
  payload.Alert = "Hello, world!"
  payload.Badge = 42
  payload.Sound = "bingbong.aiff"

  envelope := new(apns.Envelope)
  envelope.AddPayload(payload)

  alert, _ := envelope.PayloadString()
  fmt.Println(alert)
}
```

#### Returns
```json
{
  "aps": {
    "alert": "Hello, world!",
    "badge": 42,
    "sound": "bingbong.aiff"
  }
}
```

### Using an alert dictionary for complex payloads
```go
package main

import (
  "fmt"
  apns "github.com/anachronistic/apns"
)

func main() {
  args := make([]string, 1)
  args[0] = "localized args"

  dict := new(apns.AlertDictionary)
  dict.Body = "Alice wants Bob to join in the fun!"
  dict.ActionLocKey = "Play a Game!"
  dict.LocKey = "localized key"
  dict.LocArgs = args
  dict.LaunchImage = "image.jpg"

  payload := new(apns.Payload)
  payload.Alert = dict
  payload.Badge = 42
  payload.Sound = "bingbong.aiff"

  envelope := new(apns.Envelope)
  envelope.AddPayload(payload)

  alert, _ := envelope.PayloadString()
  fmt.Println(alert)
}
```

#### Returns
```json
{
  "aps": {
    "alert": {
      "body": "Alice wants Bob to join in the fun!",
      "action-loc-key": "Play a Game!",
      "loc-key": "localized key",
      "loc-args": [
        "localized args"
      ],
      "launch-image": "image.jpg"
    },
    "badge": 42,
    "sound": "bingbong.aiff"
  }
}
```

### Setting custom properties
```go
package main

import (
  "fmt"
  apns "github.com/anachronistic/apns"
)

func main() {
  payload := new(apns.Payload)
  payload.Alert = "Hello, world!"
  payload.Badge = 42
  payload.Sound = "bingbong.aiff"

  envelope := new(apns.Envelope)
  envelope.AddPayload(payload)

  envelope.Set("foo", "bar")
  envelope.Set("doctor", "who?")
  envelope.Set("the_ultimate_answer", 42)

  alert, _ := envelope.PayloadString()
  fmt.Println(alert)
}
```

#### Returns
```json
{
  "aps": {
    "alert": "Hello, world!",
    "badge": 42,
    "sound": "bingbong.aiff"
  },
  "doctor": "who?",
  "foo": "bar",
  "the_ultimate_answer": 42
}
```