package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Fremenkiel/Granny-Smith/v2/internal/handlers"
	adb "github.com/zach-klippenstein/goadb"
)

var (
	mountpath = "/Volumes/Android"
	
	port = flag.Int("p", adb.AdbPort, "")
)

func main() {
	flag.Parse()

	fh := handlers.NewFileHandler()

	ah := handlers.NewAdbHandler(fh)
	ah.Start(port)
	ah.Watch()

	
  sig := make(chan os.Signal, 1)
  signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
  <-sig
  log.Print("shutting down")
	ah.Stop()
	fh.Unmount(mountpath)
}

