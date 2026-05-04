package handlers

import (
	"log"
	"net"
	"os"
	"os/exec"

	"github.com/go-git/go-billy/v5"
	nfs "github.com/willscott/go-nfs"
	nfshelper "github.com/willscott/go-nfs/helpers"
)

type FileHandler struct {}

func NewFileHandler() *FileHandler {
	return &FileHandler{}
}

func (h *FileHandler) Mount(fs billy.Filesystem, mountpath string) {
	script := `do shell script "mkdir -p ` + mountpath + ` && chown ` + os.Getenv("USER") + ` ` + mountpath +  `" with administrator privileges`

  cmd := exec.Command("osascript", "-e", script)
  out, err := cmd.CombinedOutput()
  if err != nil {
      log.Fatalf("auth/mkdir failed: %v\n%s", err, out)
  }

	listener, err := net.Listen("tcp", "127.0.0.1:12049")
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Server running at %s\n", listener.Addr())

	handler := nfshelper.NewNullAuthHandler(fs)
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
}

func (h *FileHandler) Unmount(mountpath string) {
  exec.Command("/sbin/umount", mountpath).Run()
}
