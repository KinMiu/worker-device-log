package handlers

import (
	"device-log/services"
	"strings"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func HandleIncomingMessage(client mqtt.Client, msg mqtt.Message) {
	topicParts := strings.Split(msg.Topic(), "/")

	if len(topicParts) >= 2 {
		macaddress := topicParts[1]
		services.MarkOnline(macaddress)
	}
}
