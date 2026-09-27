package handlers

import (
	"device-log/services"
	"strings"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func HandleIncomingMessage(client mqtt.Client, msg mqtt.Message) {
	topicParts := strings.Split(msg.Topic(), "/")

	if len(topicParts) >= 2 {
		macaddress := strings.TrimSpace(topicParts[1])
		if macaddress != "" {
			services.MarkOnline(macaddress)
		}
	}
}
