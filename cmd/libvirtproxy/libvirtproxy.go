package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
)

var UnableToGetProcessName = errors.New("unable to get process name")

func processNameByPID(pid int32) (string, error) {
	exeLink := fmt.Sprintf("/proc/%d/exe", pid)
	exeName, err := os.Readlink(exeLink)
	if err != nil {
		b := make([]byte, 1024)
		// fall back to /proc/<pid>/procCmdlineFile
		procCmdlineFilename := fmt.Sprintf("/proc/%d/cmdline", pid)
		procCmdlineFile, err := os.Open(procCmdlineFilename)
		if err != nil {
			return "", fmt.Errorf("unable to open: %s", procCmdlineFilename)
		}
		n, err := procCmdlineFile.Read(b)
		if n > 0 && (err != nil || err != io.EOF) {
			sep := []byte{0}
			commands := bytes.Split(b, sep)
			return string(commands[0]), err
		}
		return "", UnableToGetProcessName
	}
	return exeName, nil
}

func accept(client *net.UnixConn) {
	defer client.Close()
	f, err := client.File()
	if err != nil {
		fmt.Println("Cannot get underlying file", err.Error())
		return
	}
	cred, err := syscall.GetsockoptUcred(int(f.Fd()), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	if err != nil {
		fmt.Println("Cannot get peer credential", err.Error())
		return
	}
	proc, err := processNameByPID(cred.Pid)
	if err != nil {
		fmt.Println("Cannot get process", err.Error())
		return
	}
	fmt.Fprintf(os.Stderr, "Credential: %+v\nProcess: %+v\n", cred, proc)
	if cred.Uid == 0 {
		fmt.Println("root client connected, do something dangerous!")
	} else {
		fmt.Println("non-root client connected, do nothing!")
	}
	//var input []byte
	input := make([]byte, 32768)
	output := make([]byte, 32768)
	fmt.Fprintf(os.Stderr, "RemoteAddress String=%s Netwok=%s\n", client.RemoteAddr().String(), client.RemoteAddr().Network())
	proxy, err := net.Dial("unix", "/run/libvirt/libvirt-sock")
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to create proxy to libvirt: %v\n", err)
		return
	}
	defer proxy.Close()
	for {
		n, err := client.Read(input)
		fmt.Printf("read %d bytes (len=%d): %v\n", n, len(input), input[:n])
		proxy.Write(input[:n])
		if err == io.EOF {
			return
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to read from socket: %v\n", err)
			return
		}

		n, err = proxy.Read(output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to read from proxy: %v\n", err)
			return
		}

		_, err = client.Write(output[:n])
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to write to socket: %v\n", err)
			return
		}
	}
}

func main() {
	args := os.Args[1:]
	if len(args) != 1 {
		binaryName := "libvirtproxy"
		exe, err := os.Executable()
		if err == nil {
			binaryName = exe
		}
		fmt.Fprintf(os.Stderr, "usage: %s <socket-file>\n", binaryName)
		os.Exit(1)
	}
	fmt.Printf("%d: %v\n", len(args), args)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{
		Name: args[0],
		Net:  args[0],
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to open listener for socket '%s': %v\n", args[0], err)
		os.Exit(2)
	}
	for {
		conn, err := listener.AcceptUnix()
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to accept connection: %v\n", err)
		}
		go accept(conn)
	}
}
