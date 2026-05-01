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

	ticker := time.NewTicker(15 * time.Second)

	go func() {
		for range ticker.C {
			services.SweepOfflineDevice()
		}
	}()

	fmt.Println("Sweeper standby each 15 seconds...")

	opts := mqtt.NewClientOptions()
	opts.AddBroker(os.Getenv("MQTT_BROKER"))
	opts.SetClientID(os.Getenv("MQTT_CLIENT_ID"))
	opts.SetUsername(os.Getenv("MQTT_USERNAME"))
	opts.SetPassword(os.Getenv("MQTT_PASSWORD"))

	opts.SetDefaultPublishHandler(handlers.HandleIncomingMessage)

	client := mqtt.NewClient(opts)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatal("Failed to connect MQTT: ", token.Error())
	}

	fmt.Println("Golang logger connected to MQTT Broker")

	topic := "sensor/#"

	if token := client.Subscribe(topic, 0, nil); token.Wait() && token.Error() != nil {
		log.Fatal("Failed to Subscribe MQTT", token.Error())
	}
	fmt.Printf("Golang service sees topic: %s\n", topic)

	keepAlive := make(chan os.Signal, 1)
	signal.Notify(keepAlive, os.Interrupt, syscall.SIGTERM)
	<-keepAlive

	fmt.Println("\nClearing the service with save mode...")
	ticker.Stop()
	client.Disconnect(250)
}
