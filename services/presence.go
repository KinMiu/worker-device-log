package services

import (
	"device-log/config"
	"fmt"
	"sync"
	"time"
)

type DeviceState struct {
	ID       string
	IsOnline bool
	LastPing time.Time
	Mu       sync.Mutex
}

var deviceCache sync.Map

func MarkOnline(macAddress string) {
	now := time.Now()

	val, exists := deviceCache.Load(macAddress)

	if exists {
		state := val.(*DeviceState)

		state.Mu.Lock()
		state.LastPing = now
		wasOffline := !state.IsOnline
		if wasOffline {
			state.IsOnline = true
		}
		state.Mu.Unlock()

		if wasOffline {
			go logStateToDB(state.ID, macAddress, true)
		}
		return
	}

	go func() {
		var id string
		query := `SELECT id FROM "Device" WHERE "macAddress" = $1`
		err := config.DB.QueryRow(query, macAddress).Scan(&id)
		if err != nil {
			return
		}

		newState := &DeviceState{
			ID:       id,
			IsOnline: true,
			LastPing: now,
		}

		deviceCache.Store(macAddress, newState)

		var lastState string
		logQuery := `SELECT state FROM "DeviceStatusLog" WHERE "deviceId" = $1 ORDER BY "createAt" DESC LIMIT 1`
		err = config.DB.QueryRow(logQuery, id).Scan(&lastState)

		if err != nil || lastState != "ONLINE" {
			logStateToDB(id, macAddress, true)
		}
	}()
}

func logStateToDB(id string, macAddress string, isOnline bool) {
	stateStr := "OFFLINE"
	reason := "Nothing data comes in during > 1 minute"

	if isOnline {
		stateStr = "ONLINE"
		reason = "The device sends data"
	}

	insertLog := `
	INSERT INTO "DeviceStatusLog" (id, "deviceId", state, reason, "createAt")
	VALUES (gen_random_uuid(), $1, $2, $3, NOW())
	`

	_, err := config.DB.Exec(insertLog, id, stateStr, reason)

	if err != nil {
		fmt.Printf("Failed insert log %s: %v\n", macAddress, err)
	}

	fmt.Printf("Logged: %s is %s\n", macAddress, stateStr)
}

func SweepOfflineDevice() {
	now := time.Now()

	deviceCache.Range(func(key, value interface{}) bool {
		macAddress := key.(string)
		state := value.(*DeviceState)

		state.Mu.Lock()
		if state.IsOnline && now.Sub(state.LastPing) > time.Minute {
			state.IsOnline = false
			state.Mu.Unlock()

			go logStateToDB(state.ID, macAddress, false)
		} else {
			state.Mu.Unlock()
		}

		return true
	})
}
