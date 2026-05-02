package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/go-git/go-billy/v5/memfs"
	nfs "github.com/willscott/go-nfs"
	nfshelper "github.com/willscott/go-nfs/helpers"
)

func main() {
	mountpath := "/Volumes/Android"
	script := `do shell script "mkdir -p ` + mountpath + ` && chown ` + os.Getenv("USER") + ` ` + mountpath +  `" with administrator privileges`
	log.Print(script)

  cmd := exec.Command("osascript", "-e", script)
  out, err := cmd.CombinedOutput()
  if err != nil {
      log.Fatalf("auth/mkdir failed: %v\n%s", err, out)
  }

	listener, err := net.Listen("tcp", "127.0.0.1:12049")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Server running at %s\n", listener.Addr())

	mem := memfs.New()
	f, err := mem.Create("hello.txt")
	if err != nil {
		log.Fatal(err)
	}

	_, err = f.Write([]byte("hello world"))
	if err != nil {
		log.Fatal(err)
	}
	f.Close()

	handler := nfshelper.NewNullAuthHandler(mem)
	cacheHelper := nfshelper.NewCachingHandler(handler, 1024)
	go func() {
		if err := nfs.Serve(listener, cacheHelper); err != nil {
			log.Fatal(err)
		}
	}()

	go exec.Command("dns-sd", "-P", "GrannySmith", "_nfs._tcp", "local",
      "12049", "grannysmith.local", "127.0.0.1").Run()

	exec.Command("/sbin/mount_nfs",
		"-o", "vers=3,nolocks,noresvport,tcp,port=12049,mountport=12049,async,rdirplus,locallocks",
		"grannysmith.local:/", mountpath,
		).Run()
	log.Print("Mount added")

	exec.Command("open", mountpath).Run()
	
  sig := make(chan os.Signal, 1)
  signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
  <-sig
  log.Print("shutting down")
  exec.Command("/sbin/umount", mountpath).Run()
}

