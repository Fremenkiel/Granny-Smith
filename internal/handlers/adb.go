package handlers

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/Fremenkiel/Granny-Smith/v2/internal/adbfs"
	"github.com/Fremenkiel/Granny-Smith/v2/internal/models"
	adb "github.com/zach-klippenstein/goadb"
)

type AdbHandler struct {
	Client				*adb.Adb
	Watcher				*adb.DeviceWatcher
	Devices			map[string]*models.Device
	FileHandler		*FileHandler
}

func NewAdbHandler(fh *FileHandler) *AdbHandler {
	return &AdbHandler{
		Devices: make(map[string]*models.Device, 0),
		FileHandler: fh,
	}
}

func (h *AdbHandler) Start(port *int) {
	client, err := adb.NewWithConfig(adb.ServerConfig{
		Port: *port,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Starting server…")
	client.StartServer()
	h.Client = client 
}

func (h *AdbHandler) Watch() {
	go func() {
		h.Watcher = h.Client.NewDeviceWatcher()
		for {
			for event := range h.Watcher.C() {
				switch event.NewState {
				case adb.StateOnline:
					err := h.addDevice(event.Serial)
					if err != nil {
						log.Print(err)
					}
				case adb.StateDisconnected:
					h.removeDevice(event.Serial)
				}
			}
			if h.Watcher.Err() != nil {
				if strings.Contains(h.Watcher.Err().Error(), "StateInvalid") {
					h.Watcher = h.Client.NewDeviceWatcher()
				} else {
					log.Print(h.Watcher.Err())
					h.Watcher.Shutdown()
				}
			}
		}
	}()
}

func (h *AdbHandler) Stop() {
	for _, d := range h.Devices {
		h.removeDevice(d.Serial)
	}

	h.Watcher.Shutdown()
	h.Watcher = nil

	h.Client.KillServer()
	h.Client = nil
}

func (h *AdbHandler) addDevice(serial string) error {
	d := h.Client.Device(adb.DeviceWithSerial(serial))
	r, err := d.Remount()
	if err != nil {
		log.Print(err)
	} else {
		log.Print(r)
	}

	di, err := d.DeviceInfo()
	if err != nil {
		return err
	}
	s, err := d.Serial()
	if err != nil {
		return err
	}
	fs := adbfs.New(d)

	ad := &models.Device{Serial: s, Name: di.Model, MountDir: "/Volumes/" + di.Model }
	h.FileHandler.Mount(fs, ad.MountDir)

	h.Devices[s] = ad
	log.Printf("Adding: %s - %s", di.Model, s)
	return nil
}

func (h *AdbHandler) removeDevice(serial string) {
	d := h.Devices[serial]
	log.Printf("Removing: %s", d.Name)

	delete(h.Devices, serial)
	h.FileHandler.Unmount(d.MountDir)
	log.Printf("New slice length %s", strconv.Itoa(len(h.Devices)))
}

