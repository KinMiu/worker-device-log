package main

import (
	"device-log/config"
	"device-log/handlers"
	"device-log/services"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()

	if err != nil {
		log.Println("File .env not found")
	}

	config.ConnectDB()
	services.InitPresenceService()

	ticker := time.NewTicker(15 * time.Second)

	go func() {
		for range ticker.C {
			services.SweepOfflineDevice()
		}
	}()

	fmt.Println("Sweeper standby each 15 seconds...")

	clientID := os.Getenv("MQTT_CLIENT_ID")
	if clientID == "" {
		clientID = "golang_logger"
	}
	uniqueClientID := fmt.Sprintf("%s_%d", clientID, time.Now().UnixNano()%1000000)

	topic := "sensor/#"

	opts := mqtt.NewClientOptions()
	opts.AddBroker(os.Getenv("MQTT_BROKER"))
	opts.SetClientID(uniqueClientID)
	opts.SetUsername(os.Getenv("MQTT_USERNAME"))
	opts.SetPassword(os.Getenv("MQTT_PASSWORD"))
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(5 * time.Second)
	opts.SetKeepAlive(30 * time.Second)
	opts.SetPingTimeout(10 * time.Second)
	opts.SetConnectTimeout(10 * time.Second)

	opts.SetDefaultPublishHandler(handlers.HandleIncomingMessage)

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Printf("Golang logger connected/reconnected to MQTT Broker (ClientID: %s)\n", uniqueClientID)
		if token := c.Subscribe(topic, 0, handlers.HandleIncomingMessage); token.Wait() && token.Error() != nil {
			log.Printf("Failed to Subscribe MQTT (%s): %v\n", topic, token.Error())
		} else {
			log.Printf("Golang service subscribed to topic: %s\n", topic)
		}
	})

	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		log.Printf("MQTT Connection lost: %v. Auto-reconnecting...\n", err)
	})

	client := mqtt.NewClient(opts)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatal("Failed to connect MQTT: ", token.Error())
	}

	keepAlive := make(chan os.Signal, 1)
	signal.Notify(keepAlive, os.Interrupt, syscall.SIGTERM)
	<-keepAlive

	fmt.Println("\nClearing the service with save mode...")
	ticker.Stop()
	client.Disconnect(250)
	services.ClosePresenceService()
}
