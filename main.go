package main

import (
	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/certificate"
	"tailscale.com/tsnet"
)

func main() {
    cert, err := certificate.FromP12File("", "")
    notification := &apns2.Notification{}

    s := new(tsnet.Server)

    _, _, _, _ = cert, err, notification, s
}
